package backend

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const backupManifestName = "manifest.json"

func writeBackupArchive(path string, backup BackupEnvelope, store *Store) error {
	if filepath.Ext(path) == "" {
		path += ".todo-backup.zip"
	}
	temp := path + ".tmp"
	_ = os.Remove(temp)
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建备份失败: %w", err)
	}
	writer := zip.NewWriter(file)
	closeWithError := func(cause error) error {
		_ = writer.Close()
		_ = file.Close()
		_ = os.Remove(temp)
		return cause
	}
	manifest, err := writer.Create(backupManifestName)
	if err != nil {
		return closeWithError(err)
	}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return closeWithError(err)
	}
	if _, err := manifest.Write(data); err != nil {
		return closeWithError(err)
	}
	for _, attachment := range backup.Attachments {
		_, sourcePath, err := store.attachmentPath(context.Background(), attachment.ID)
		if err != nil {
			return closeWithError(err)
		}
		source, err := os.Open(sourcePath)
		if err != nil {
			return closeWithError(err)
		}
		entryName := filepath.ToSlash(filepath.Join("attachments", attachment.ID, filepath.Base(attachment.FileName)))
		entry, err := writer.Create(entryName)
		if err == nil {
			_, err = io.Copy(entry, source)
		}
		source.Close()
		if err != nil {
			return closeWithError(err)
		}
	}
	if err := writer.Close(); err != nil {
		file.Close()
		os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(path)
		if err := os.Rename(temp, path); err != nil {
			return fmt.Errorf("保存备份失败: %w", err)
		}
	}
	return nil
}

func readBackupArchive(path string, store *Store) (BackupEnvelope, map[string]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return BackupEnvelope{}, nil, fmt.Errorf("读取备份失败: %w", readErr)
		}
		var legacy BackupEnvelope
		if jsonErr := json.Unmarshal(data, &legacy); jsonErr != nil {
			return BackupEnvelope{}, nil, errors.New("备份文件不是有效的 TODO 备份")
		}
		return legacy, nil, nil
	}
	defer reader.Close()
	var backup BackupEnvelope
	entries := map[string]*zip.File{}
	for _, file := range reader.File {
		clean := filepath.ToSlash(filepath.Clean(file.Name))
		if strings.HasPrefix(clean, "../") || filepath.IsAbs(file.Name) {
			return BackupEnvelope{}, nil, errors.New("备份包含不安全的文件路径")
		}
		entries[clean] = file
	}
	manifest := entries[backupManifestName]
	if manifest == nil || manifest.UncompressedSize64 > 32*1024*1024 {
		return BackupEnvelope{}, nil, errors.New("备份缺少有效清单")
	}
	manifestReader, err := manifest.Open()
	if err != nil {
		return BackupEnvelope{}, nil, err
	}
	data, err := io.ReadAll(io.LimitReader(manifestReader, 32*1024*1024+1))
	manifestReader.Close()
	if err != nil || len(data) > 32*1024*1024 {
		return BackupEnvelope{}, nil, errors.New("备份清单过大或读取失败")
	}
	if err := json.Unmarshal(data, &backup); err != nil {
		return BackupEnvelope{}, nil, errors.New("备份清单不是有效的 JSON")
	}
	if err := validateSnapshot(backup); err != nil {
		return BackupEnvelope{}, nil, err
	}
	paths := map[string]string{}
	staging := filepath.Join(store.attachmentDir, ".import-"+uuid.NewString())
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return BackupEnvelope{}, nil, err
	}
	defer os.RemoveAll(staging)
	for _, attachment := range backup.Attachments {
		prefix := "attachments/" + attachment.ID + "/"
		var entry *zip.File
		for name, candidate := range entries {
			if strings.HasPrefix(name, prefix) && !strings.HasSuffix(name, "/") {
				entry = candidate
				break
			}
		}
		if entry == nil || int64(entry.UncompressedSize64) != attachment.Size || entry.UncompressedSize64 > uint64(maxAttachmentBytes) {
			return BackupEnvelope{}, nil, fmt.Errorf("附件 %s 缺失或大小无效", attachment.FileName)
		}
		source, err := entry.Open()
		if err != nil {
			return BackupEnvelope{}, nil, err
		}
		tempPath := filepath.Join(staging, attachment.ID)
		target, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			source.Close()
			return BackupEnvelope{}, nil, err
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(target, hash), io.LimitReader(source, maxAttachmentBytes+1))
		source.Close()
		closeErr := target.Close()
		if copyErr != nil || closeErr != nil || written != attachment.Size || fmt.Sprintf("%x", hash.Sum(nil)) != attachment.SHA256 {
			return BackupEnvelope{}, nil, fmt.Errorf("附件 %s 校验失败", attachment.FileName)
		}
		destinationDir := filepath.Join(store.attachmentDir, attachment.NotebookID)
		if err := os.MkdirAll(destinationDir, 0o700); err != nil {
			return BackupEnvelope{}, nil, err
		}
		destination := filepath.Join(destinationDir, attachment.SHA256+strings.ToLower(filepath.Ext(attachment.FileName)))
		if _, err := os.Stat(destination); errors.Is(err, os.ErrNotExist) {
			if err := copyFile(tempPath, destination); err != nil {
				return BackupEnvelope{}, nil, err
			}
		}
		paths[attachment.ID] = destination
	}
	return backup, paths, nil
}

func copyFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		os.Remove(destinationPath)
		return err
	}
	return target.Close()
}
