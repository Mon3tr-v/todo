package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/zalando/go-keyring"
)

const credentialService = "TODO Sync"

type syncTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
	UserID       string `json:"userId"`
	Username     string `json:"username"`
}

type SyncClient struct {
	store     *Store
	http      *http.Client
	mu        sync.Mutex
	serverURL string
	tokens    syncTokens
	onStatus  func(SyncStatus)
}

func NewSyncClient(store *Store, onStatus func(SyncStatus)) *SyncClient {
	return &SyncClient{store: store, http: &http.Client{Timeout: 45 * time.Second}, onStatus: onStatus}
}

func (c *SyncClient) RestoreSession(ctx context.Context) error {
	state, err := c.syncState(ctx)
	if err != nil || state["server_url"] == "" || state["username"] == "" {
		return err
	}
	key := credentialKey(state["server_url"], state["username"])
	value, err := keyring.Get(credentialService, key)
	if err != nil {
		return nil
	}
	var tokens syncTokens
	if err := json.Unmarshal([]byte(value), &tokens); err != nil {
		return err
	}
	c.mu.Lock()
	c.serverURL, c.tokens = strings.TrimRight(state["server_url"], "/"), tokens
	c.mu.Unlock()
	return nil
}

func validateServerURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("同步服务器地址无效")
	}
	localhost := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(localhost && parsed.Scheme == "http") {
		return "", errors.New("非本机同步服务器必须使用 HTTPS")
	}
	return value, nil
}

func (c *SyncClient) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	serverURL, err := validateServerURL(input.ServerURL)
	if err != nil {
		return LoginResult{}, err
	}
	body := map[string]string{"username": strings.TrimSpace(input.Username), "password": input.Password}
	var response struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
		User         struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, serverURL+"/api/v1/auth/login", "", body, &response); err != nil {
		return LoginResult{}, err
	}
	if response.User.ID == "" {
		return LoginResult{}, errors.New("同步服务器返回了无效账号")
	}
	tokens := syncTokens{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(response.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano), UserID: response.User.ID, Username: response.User.Username}
	encoded, _ := json.Marshal(tokens)
	if err := keyring.Set(credentialService, credentialKey(serverURL, tokens.Username), string(encoded)); err != nil {
		return LoginResult{}, fmt.Errorf("无法将登录令牌保存到 Windows 凭据管理器: %w", err)
	}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return LoginResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sync_state`); err != nil {
		return LoginResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM outbox`); err != nil {
		return LoginResult{}, err
	}
	for key, value := range map[string]string{"server_url": serverURL, "username": tokens.Username, "user_id": tokens.UserID, "last_error": ""} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sync_state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return LoginResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return LoginResult{}, err
	}
	c.mu.Lock()
	c.serverURL, c.tokens = serverURL, tokens
	c.mu.Unlock()
	if err := c.QueueFullSync(ctx); err != nil {
		return LoginResult{}, err
	}
	_ = c.SyncNow(ctx)
	status, _ := c.store.GetSyncStatus(ctx)
	return LoginResult{User: AccountUser{ID: tokens.UserID, Username: tokens.Username, Mode: "sync"}, Sync: status}, nil
}

func credentialKey(serverURL, username string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "@" + strings.ToLower(strings.TrimSpace(serverURL))
}

func (c *SyncClient) Logout(ctx context.Context) error {
	c.mu.Lock()
	serverURL, tokens := c.serverURL, c.tokens
	c.serverURL, c.tokens = "", syncTokens{}
	c.mu.Unlock()
	if serverURL != "" && tokens.RefreshToken != "" {
		_ = c.requestJSON(ctx, http.MethodPost, serverURL+"/api/v1/auth/logout", tokens.AccessToken, map[string]string{"refreshToken": tokens.RefreshToken}, nil)
		_ = keyring.Delete(credentialService, credentialKey(serverURL, tokens.Username))
	}
	_, err := c.store.db.ExecContext(ctx, `DELETE FROM sync_state`)
	if err == nil {
		_, err = c.store.db.ExecContext(ctx, `DELETE FROM outbox`)
	}
	c.emitStatus(ctx)
	return err
}

func (c *SyncClient) requestJSON(ctx context.Context, method, endpoint, access string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if access != "" {
		request.Header.Set("Authorization", "Bearer "+access)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("无法连接同步服务器: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var remote struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &remote)
		if remote.Message == "" {
			remote.Message = fmt.Sprintf("同步服务器返回状态 %d", response.StatusCode)
		}
		return &remoteError{Status: response.StatusCode, Message: remote.Message}
	}
	if output != nil && len(data) > 0 {
		return json.Unmarshal(data, output)
	}
	return nil
}

type remoteError struct {
	Status  int
	Message string
}

func (e *remoteError) Error() string { return e.Message }

func (c *SyncClient) refresh(ctx context.Context) error {
	c.mu.Lock()
	serverURL, tokens := c.serverURL, c.tokens
	c.mu.Unlock()
	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, serverURL+"/api/v1/auth/refresh", "", map[string]string{"refreshToken": tokens.RefreshToken}, &result); err != nil {
		return err
	}
	tokens.AccessToken, tokens.RefreshToken = result.AccessToken, result.RefreshToken
	tokens.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano)
	encoded, _ := json.Marshal(tokens)
	if err := keyring.Set(credentialService, credentialKey(serverURL, tokens.Username), string(encoded)); err != nil {
		return err
	}
	c.mu.Lock()
	c.tokens = tokens
	c.mu.Unlock()
	return nil
}

func (c *SyncClient) authenticatedRequest(ctx context.Context, method, path string, input, output any) error {
	c.mu.Lock()
	serverURL, tokens := c.serverURL, c.tokens
	c.mu.Unlock()
	if serverURL == "" || tokens.AccessToken == "" {
		return errors.New("尚未配置同步账号")
	}
	err := c.requestJSON(ctx, method, serverURL+path, tokens.AccessToken, input, output)
	var remote *remoteError
	if errors.As(err, &remote) && remote.Status == http.StatusUnauthorized {
		if refreshErr := c.refresh(ctx); refreshErr != nil {
			return refreshErr
		}
		c.mu.Lock()
		tokens = c.tokens
		c.mu.Unlock()
		return c.requestJSON(ctx, method, serverURL+path, tokens.AccessToken, input, output)
	}
	return err
}

func (c *SyncClient) QueueFullSync(ctx context.Context) error {
	backup, err := c.store.ExportSnapshot(ctx)
	if err != nil {
		return err
	}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, notebook := range backup.Notebooks {
		if err := forceOutboxTx(ctx, tx, "notebook", notebook.ID, "upsert", notebook.ServerVersion, notebook); err != nil {
			return err
		}
	}
	for _, list := range backup.Lists {
		if err := forceOutboxTx(ctx, tx, "list", list.ID, "upsert", 0, list); err != nil {
			return err
		}
	}
	for _, tag := range backup.Tags {
		if err := forceOutboxTx(ctx, tx, "tag", tag.ID, "upsert", 0, tag); err != nil {
			return err
		}
	}
	for _, task := range backup.Tasks {
		if err := forceOutboxTx(ctx, tx, "task", task.ID, "upsert", 0, task); err != nil {
			return err
		}
	}
	for _, tag := range backup.NoteTags {
		if err := forceOutboxTx(ctx, tx, "note_tag", tag.ID, "upsert", 0, tag); err != nil {
			return err
		}
	}
	for _, attachment := range backup.Attachments {
		if err := forceOutboxTx(ctx, tx, "attachment", attachment.ID, "upsert", 0, attachment); err != nil {
			return err
		}
	}
	for _, note := range backup.Notes {
		note.TaskIDs = nil
		if err := forceOutboxTx(ctx, tx, "note", note.ID, "upsert", 0, note); err != nil {
			return err
		}
	}
	for _, comment := range backup.Comments {
		if err := forceOutboxTx(ctx, tx, "comment", comment.ID, "upsert", 0, comment); err != nil {
			return err
		}
	}
	for _, version := range backup.Versions {
		if err := forceOutboxTx(ctx, tx, "note_version", version.ID, "upsert", 0, version); err != nil {
			return err
		}
	}
	if err := forceOutboxTx(ctx, tx, "settings", localUserID, "upsert", 0, backup.Settings); err != nil {
		return err
	}
	return tx.Commit()
}

func forceOutboxTx(ctx context.Context, tx *sql.Tx, entityType, entityID, action string, baseVersion int64, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(id,entity_type,entity_id,action,base_version,payload,created_at) VALUES(?,?,?,?,?,?,?)`, uuid.NewString(), entityType, entityID, action, baseVersion, string(data), nowString())
	return err
}

type queuedOperation struct {
	ID          string          `json:"id"`
	EntityType  string          `json:"entityType"`
	EntityID    string          `json:"entityId"`
	Action      string          `json:"action"`
	BaseVersion int64           `json:"baseVersion"`
	Payload     json.RawMessage `json:"payload"`
}

type remoteChange struct {
	Sequence   int64           `json:"sequence"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Action     string          `json:"action"`
	Version    int64           `json:"version"`
	Payload    json.RawMessage `json:"payload"`
}

type syncBootstrapResponse struct {
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Notebooks     []Notebook       `json:"notebooks"`
	Memberships   []NotebookMember `json:"memberships"`
	Entities      []remoteChange   `json:"entities"`
	Notifications []Notification   `json:"notifications"`
	Cursor        string           `json:"cursor"`
	UnreadCount   int              `json:"unreadCount"`
}

func (c *SyncClient) SyncNow(ctx context.Context) error {
	c.mu.Lock()
	configured := c.serverURL != "" && c.tokens.AccessToken != ""
	c.mu.Unlock()
	if !configured {
		return errors.New("尚未配置同步账号")
	}
	_ = c.setSyncValue(ctx, "last_error", "")
	if err := c.push(ctx); err != nil {
		_ = c.setSyncValue(ctx, "last_error", err.Error())
		c.emitStatus(ctx)
		return err
	}
	attachmentErr := c.uploadPendingAttachments(ctx)
	state, _ := c.syncState(ctx)
	if state["cursor"] == "" {
		if err := c.bootstrap(ctx); err != nil {
			_ = c.setSyncValue(ctx, "last_error", err.Error())
			c.emitStatus(ctx)
			return err
		}
	} else if err := c.pull(ctx); err != nil {
		_ = c.setSyncValue(ctx, "last_error", err.Error())
		c.emitStatus(ctx)
		return err
	}
	if err := c.pullNotifications(ctx); err != nil {
		_ = c.setSyncValue(ctx, "last_error", err.Error())
		c.emitStatus(ctx)
		return err
	}
	_ = c.setSyncValue(ctx, "last_sync_at", nowString())
	if attachmentErr != nil {
		_ = c.setSyncValue(ctx, "last_error", attachmentErr.Error())
	} else {
		_ = c.setSyncValue(ctx, "last_error", "")
	}
	c.emitStatus(ctx)
	return attachmentErr
}

func (c *SyncClient) uploadPendingAttachments(ctx context.Context) error {
	rows, err := c.store.db.QueryContext(ctx, `SELECT id,notebook_id,file_name,mime_type,size_bytes,sha256,local_path FROM attachments
		WHERE deleted_at IS NULL AND status IN ('local','pending','failed') ORDER BY created_at LIMIT 20`)
	if err != nil {
		return err
	}
	type pendingAttachment struct {
		id, notebookID, fileName, mimeType, sha, path string
		size                                          int64
	}
	items := []pendingAttachment{}
	for rows.Next() {
		var item pendingAttachment
		if err := rows.Scan(&item.id, &item.notebookID, &item.fileName, &item.mimeType, &item.size, &item.sha, &item.path); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var lastErr error
	for _, item := range items {
		var prepared struct {
			AttachmentID string `json:"attachmentId"`
			UploadURL    string `json:"uploadUrl"`
		}
		input := map[string]any{"id": item.id, "notebookId": item.notebookID, "fileName": item.fileName, "mimeType": item.mimeType, "size": item.size, "sha256": item.sha}
		if err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/attachments/prepare", input, &prepared); err != nil {
			lastErr = fmt.Errorf("附件 %s 同步失败: %w", item.fileName, err)
			_, _ = c.store.db.ExecContext(ctx, `UPDATE attachments SET status='failed',updated_at=? WHERE id=?`, nowString(), item.id)
			continue
		}
		file, err := os.Open(item.path)
		if err != nil {
			lastErr = err
			continue
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, prepared.UploadURL, file)
		if err == nil {
			request.ContentLength = item.size
			request.Header.Set("Content-Type", item.mimeType)
			var response *http.Response
			response, err = c.http.Do(request)
			if err == nil {
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					err = fmt.Errorf("object storage returned %d", response.StatusCode)
				}
			}
		}
		file.Close()
		if err != nil {
			lastErr = fmt.Errorf("附件 %s 上传失败: %w", item.fileName, err)
			_, _ = c.store.db.ExecContext(ctx, `UPDATE attachments SET status='failed',updated_at=? WHERE id=?`, nowString(), item.id)
			continue
		}
		if err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/attachments/complete", map[string]string{"attachmentId": item.id}, nil); err != nil {
			lastErr = err
			continue
		}
		_, _ = c.store.db.ExecContext(ctx, `UPDATE attachments SET status='synced',updated_at=? WHERE id=?`, nowString(), item.id)
	}
	return lastErr
}

func (c *SyncClient) push(ctx context.Context) error {
	rows, err := c.store.db.QueryContext(ctx, `SELECT id,entity_type,entity_id,action,base_version,payload FROM outbox ORDER BY created_at LIMIT 100`)
	if err != nil {
		return err
	}
	operations := []queuedOperation{}
	for rows.Next() {
		var item queuedOperation
		var payload string
		if err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.Action, &item.BaseVersion, &payload); err != nil {
			rows.Close()
			return err
		}
		item.Payload = json.RawMessage(payload)
		operations = append(operations, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(operations) == 0 {
		return nil
	}
	var response struct {
		Results []struct {
			ID, EntityID, ConflictCopyID string
			Version                      int64
		} `json:"results"`
	}
	if err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/sync/push", map[string]any{"operations": operations}, &response); err != nil {
		return err
	}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, result := range response.Results {
		if _, err := tx.ExecContext(ctx, `DELETE FROM outbox WHERE id=?`, result.ID); err != nil {
			return err
		}
		if result.Version > 0 {
			switch operationsByID(operations, result.ID).EntityType {
			case "note":
				_, _ = tx.ExecContext(ctx, `UPDATE notes SET server_version=? WHERE id=?`, result.Version, operationsByID(operations, result.ID).EntityID)
			case "notebook":
				_, _ = tx.ExecContext(ctx, `UPDATE notebooks SET server_version=? WHERE id=?`, result.Version, operationsByID(operations, result.ID).EntityID)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if len(operations) == 100 {
		return c.push(ctx)
	}
	return nil
}

func operationsByID(items []queuedOperation, id string) queuedOperation {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return queuedOperation{}
}

func (c *SyncClient) pull(ctx context.Context) error {
	state, err := c.syncState(ctx)
	if err != nil {
		return err
	}
	cursor := state["cursor"]
	if cursor == "" {
		cursor = "0"
	}
	for {
		var response struct {
			Cursor  string         `json:"cursor"`
			HasMore bool           `json:"hasMore"`
			Changes []remoteChange `json:"changes"`
		}
		if err := c.authenticatedRequest(ctx, http.MethodGet, "/api/v1/sync/pull?cursor="+url.QueryEscape(cursor)+"&limit=500", nil, &response); err != nil {
			var remote *remoteError
			if errors.As(err, &remote) && remote.Status == http.StatusConflict {
				return c.bootstrap(ctx)
			}
			return err
		}
		tx, err := c.store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, change := range response.Changes {
			if err := applyRemoteChangeTx(ctx, tx, change.EntityType, change.EntityID, change.Action, change.Version, change.Payload, c.store.attachmentDir); err != nil {
				tx.Rollback()
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sync_state(key,value) VALUES('cursor',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, response.Cursor); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		cursor = response.Cursor
		if !response.HasMore {
			return nil
		}
	}
}

func (c *SyncClient) bootstrap(ctx context.Context) error {
	var response syncBootstrapResponse
	if err := c.authenticatedRequest(ctx, http.MethodGet, "/api/v1/sync/bootstrap", nil, &response); err != nil {
		return err
	}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowString()
	accessible := make(map[string]bool, len(response.Notebooks))
	remoteVersions := map[string]bool{}
	for _, entity := range response.Entities {
		if entity.EntityType == "note_version" && entity.Action != "delete" {
			remoteVersions[entity.EntityID] = true
		}
	}
	for _, notebook := range response.Notebooks {
		accessible[notebook.ID] = true
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM notebooks WHERE server_version>0 AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	remoteIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		remoteIDs = append(remoteIDs, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range remoteIDs {
		if !accessible[id] {
			if _, err := tx.ExecContext(ctx, `UPDATE notebooks SET deleted_at=?,updated_at=? WHERE id=?`, now, now, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM memberships WHERE notebook_id=?`, id); err != nil {
				return err
			}
		}
	}
	versionRows, err := tx.QueryContext(ctx, `SELECT v.id FROM note_versions v JOIN notes n ON n.id=v.note_id JOIN notebooks nb ON nb.id=n.notebook_id
		WHERE nb.server_version>0 AND NOT EXISTS(SELECT 1 FROM outbox o WHERE o.entity_type='note_version' AND o.entity_id=v.id)`)
	if err != nil {
		return err
	}
	localVersionIDs := []string{}
	for versionRows.Next() {
		var id string
		if err := versionRows.Scan(&id); err != nil {
			versionRows.Close()
			return err
		}
		localVersionIDs = append(localVersionIDs, id)
	}
	if err := versionRows.Close(); err != nil {
		return err
	}
	for _, id := range localVersionIDs {
		if !remoteVersions[id] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM note_versions WHERE id=?`, id); err != nil {
				return err
			}
		}
	}
	for _, notebook := range response.Notebooks {
		kind := notebook.Kind
		if kind != "shared" {
			kind = "personal"
		}
		role := notebook.Role
		if role != "owner" && role != "editor" && role != "viewer" {
			role = "viewer"
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO notebooks(id,name,kind,role,owner_id,is_default,deleted_at,server_version,created_at,updated_at)
			VALUES(?,?,?,?,?,0,NULL,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,kind=excluded.kind,role=excluded.role,
			owner_id=excluded.owner_id,deleted_at=NULL,server_version=excluded.server_version,updated_at=excluded.updated_at`,
			notebook.ID, coalesce(notebook.Name, "共享笔记本"), kind, role, coalesce(notebook.OwnerID, "remote"), notebook.ServerVersion,
			coalesce(notebook.CreatedAt, now), coalesce(notebook.UpdatedAt, now))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM memberships WHERE notebook_id=?`, notebook.ID); err != nil {
			return err
		}
	}
	for _, member := range response.Memberships {
		if _, err := tx.ExecContext(ctx, `INSERT INTO memberships(notebook_id,user_id,username,role,created_at) VALUES(?,?,?,?,?)
			ON CONFLICT(notebook_id,user_id) DO UPDATE SET username=excluded.username,role=excluded.role`, member.NotebookID, member.UserID,
			member.Username, member.Role, coalesce(member.CreatedAt, now)); err != nil {
			return err
		}
	}
	for _, change := range response.Entities {
		if change.EntityType == "notebook" {
			continue
		}
		if err := applyRemoteChangeTx(ctx, tx, change.EntityType, change.EntityID, change.Action, change.Version, change.Payload, c.store.attachmentDir); err != nil {
			return err
		}
	}
	if err := upsertNotificationsTx(ctx, tx, response.Notifications); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sync_state(key,value) VALUES('cursor',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, response.Cursor); err != nil {
		return err
	}
	return tx.Commit()
}

func applyRemoteChangeTx(ctx context.Context, tx *sql.Tx, entityType, entityID, action string, version int64, payload json.RawMessage, attachmentDir string) error {
	deleted := action == "delete"
	switch entityType {
	case "notebook":
		var item Notebook
		if json.Unmarshal(payload, &item) != nil {
			return nil
		}
		if item.Name == "" {
			item.Name = "共享笔记本"
		}
		now := nowString()
		deletedAt := any(nil)
		if deleted {
			deletedAt = now
		}
		kind := item.Kind
		if kind != "personal" && kind != "shared" {
			kind = "shared"
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO notebooks(id,name,kind,role,owner_id,is_default,deleted_at,server_version,created_at,updated_at) VALUES(?,?,?,?,?,0,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,deleted_at=excluded.deleted_at,server_version=excluded.server_version,updated_at=excluded.updated_at`, entityID, item.Name, kind, coalesce(item.Role, "editor"), coalesce(item.OwnerID, "remote"), deletedAt, version, coalesce(item.CreatedAt, now), now)
		return err
	case "note":
		var item BackupNote
		if json.Unmarshal(payload, &item) != nil {
			return nil
		}
		if item.NotebookID == "" {
			return nil
		}
		title, _ := normalizeNoteTitle(item.Title)
		now := nowString()
		deletedAt := any(nil)
		if deleted {
			deletedAt = now
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO notes(id,notebook_id,title,content,pinned,revision,deleted_at,server_version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET notebook_id=excluded.notebook_id,title=excluded.title,content=excluded.content,pinned=excluded.pinned,revision=MAX(notes.revision,excluded.revision),deleted_at=excluded.deleted_at,server_version=excluded.server_version,updated_at=excluded.updated_at`, entityID, item.NotebookID, title, item.Content, boolInt(item.Pinned), maxInt64(item.Revision, 1), deletedAt, version, coalesce(item.CreatedAt, now), coalesce(item.UpdatedAt, now))
		if err != nil {
			return err
		}
		if !deleted {
			_ = replaceRemoteNoteLinksTx(ctx, tx, entityID, item.NotebookID, item.TagIDs, item.AttachmentIDs)
		}
		return nil
	case "note_tag":
		var item NoteTag
		if json.Unmarshal(payload, &item) != nil || item.NotebookID == "" {
			return nil
		}
		now := nowString()
		deletedAt := any(nil)
		if deleted {
			deletedAt = now
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO note_tags(id,notebook_id,name,color,deleted_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,color=excluded.color,deleted_at=excluded.deleted_at,updated_at=excluded.updated_at`, entityID, item.NotebookID, item.Name, normalizeTagColor(item.Color), deletedAt, coalesce(item.CreatedAt, now), now)
		return err
	case "attachment":
		var item Attachment
		if json.Unmarshal(payload, &item) != nil || item.NotebookID == "" || item.SHA256 == "" {
			return nil
		}
		now := nowString()
		deletedAt := any(nil)
		if deleted {
			deletedAt = now
		}
		localPath := filepath.Join(attachmentDir, item.NotebookID, item.SHA256+strings.ToLower(filepath.Ext(item.FileName)))
		_, err := tx.ExecContext(ctx, `INSERT INTO attachments(id,notebook_id,file_name,mime_type,size_bytes,sha256,local_path,remote_key,status,deleted_at,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?, 'synced',?,?,?) ON CONFLICT(id) DO UPDATE SET file_name=excluded.file_name,mime_type=excluded.mime_type,
			size_bytes=excluded.size_bytes,sha256=excluded.sha256,remote_key=excluded.remote_key,status='synced',deleted_at=excluded.deleted_at,updated_at=excluded.updated_at`,
			entityID, item.NotebookID, item.FileName, item.MimeType, item.Size, item.SHA256, localPath, item.RemoteKey, deletedAt, coalesce(item.CreatedAt, now), coalesce(item.UpdatedAt, now))
		return err
	case "task":
		var item Task
		if json.Unmarshal(payload, &item) != nil {
			return nil
		}
		now := nowString()
		var listID, dueDate, completedAt, deletedAt any
		if item.ListID != "" {
			listID = item.ListID
		}
		if item.DueDate != "" {
			dueDate = item.DueDate
		}
		if item.CompletedAt != "" {
			completedAt = item.CompletedAt
		}
		if deleted {
			deletedAt = now
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO tasks(id,title,notes,list_id,priority,due_date,completed_at,deleted_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET title=excluded.title,notes=excluded.notes,list_id=excluded.list_id,priority=excluded.priority,due_date=excluded.due_date,completed_at=excluded.completed_at,deleted_at=excluded.deleted_at,updated_at=excluded.updated_at`, entityID, item.Title, item.Notes, listID, item.Priority, dueDate, completedAt, deletedAt, coalesce(item.CreatedAt, now), now)
		return err
	case "list":
		var item TodoList
		if json.Unmarshal(payload, &item) != nil {
			return nil
		}
		if deleted {
			_, err := tx.ExecContext(ctx, `DELETE FROM lists WHERE id=?`, entityID)
			return err
		}
		now := nowString()
		_, err := tx.ExecContext(ctx, `INSERT INTO lists(id,name,position,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,position=excluded.position,updated_at=excluded.updated_at`, entityID, item.Name, item.Position, coalesce(item.CreatedAt, now), now)
		return err
	case "tag":
		var item Tag
		if json.Unmarshal(payload, &item) != nil {
			return nil
		}
		if deleted {
			_, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, entityID)
			return err
		}
		now := nowString()
		_, err := tx.ExecContext(ctx, `INSERT INTO tags(id,name,color,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,color=excluded.color,updated_at=excluded.updated_at`, entityID, item.Name, item.Color, coalesce(item.CreatedAt, now), now)
		return err
	case "settings":
		var values map[string]string
		if json.Unmarshal(payload, &values) != nil {
			return nil
		}
		for key, value := range values {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
				return err
			}
		}
	case "comment":
		var item Comment
		if json.Unmarshal(payload, &item) != nil || item.NoteID == "" {
			return nil
		}
		if deleted {
			_, err := tx.ExecContext(ctx, `UPDATE comments SET deleted_at=?,updated_at=? WHERE id=?`, nowString(), nowString(), entityID)
			return err
		}
		var parent any
		if item.ParentID != "" {
			parent = item.ParentID
		}
		now := nowString()
		_, err := tx.ExecContext(ctx, `INSERT INTO comments(id,note_id,parent_id,author_id,author_name,content,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET content=excluded.content,author_id=excluded.author_id,author_name=excluded.author_name,deleted_at=NULL,updated_at=excluded.updated_at`, entityID, item.NoteID, parent, coalesce(item.AuthorID, "remote"), coalesce(item.AuthorName, "协作者"), item.Content, coalesce(item.CreatedAt, now), coalesce(item.UpdatedAt, now))
		return err
	case "note_version":
		var item BackupNoteVersion
		if json.Unmarshal(payload, &item) != nil || item.NoteID == "" {
			return nil
		}
		if deleted {
			_, err := tx.ExecContext(ctx, `DELETE FROM note_versions WHERE id=?`, entityID)
			return err
		}
		tags, _ := json.Marshal(uniqueStrings(item.TagIDs))
		attachments, _ := json.Marshal(uniqueStrings(item.AttachmentIDs))
		_, err := tx.ExecContext(ctx, `INSERT INTO note_versions(id,note_id,title,content,tag_ids,attachment_ids,actor_id,actor_name,source_revision,created_at)
			SELECT ?,id,?,?,?,?,?,?,?,? FROM notes WHERE id=? ON CONFLICT(id) DO UPDATE SET title=excluded.title,content=excluded.content,
			tag_ids=excluded.tag_ids,attachment_ids=excluded.attachment_ids,actor_id=excluded.actor_id,actor_name=excluded.actor_name,
			source_revision=excluded.source_revision,created_at=excluded.created_at`, entityID, item.Title, item.Content, string(tags), string(attachments),
			coalesce(item.ActorID, "remote"), coalesce(item.ActorName, "协作者"), item.SourceRevision, coalesce(item.CreatedAt, nowString()), item.NoteID)
		return err
	}
	return nil
}

func replaceRemoteNoteLinksTx(ctx context.Context, tx *sql.Tx, noteID, notebookID string, tagIDs, attachmentIDs []string) error {
	_, _ = tx.ExecContext(ctx, `DELETE FROM note_tag_links WHERE note_id=?`, noteID)
	for _, id := range uniqueStrings(tagIDs) {
		_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO note_tag_links(note_id,tag_id) SELECT ?,id FROM note_tags WHERE id=? AND notebook_id=?`, noteID, id, notebookID)
	}
	_, _ = tx.ExecContext(ctx, `DELETE FROM note_attachments WHERE note_id=?`, noteID)
	for position, id := range uniqueStrings(attachmentIDs) {
		_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO note_attachments(note_id,attachment_id,position) SELECT ?,id,? FROM attachments WHERE id=? AND deleted_at IS NULL`, noteID, position, id)
	}
	return nil
}
func coalesce(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (c *SyncClient) syncState(ctx context.Context) (map[string]string, error) {
	rows, err := c.store.db.QueryContext(ctx, `SELECT key,value FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, rows.Err()
}
func (c *SyncClient) setSyncValue(ctx context.Context, key, value string) error {
	_, err := c.store.db.ExecContext(ctx, `INSERT INTO sync_state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
func (c *SyncClient) emitStatus(ctx context.Context) {
	if c.onStatus == nil {
		return
	}
	status, _ := c.store.GetSyncStatus(ctx)
	c.onStatus(status)
}

func (c *SyncClient) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.SyncNow(ctx)
		}
	}
}

func (c *SyncClient) RunWebSocket(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		serverURL, tokens := c.serverURL, c.tokens
		c.mu.Unlock()
		if serverURL == "" || tokens.AccessToken == "" {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}
		wsURL := strings.Replace(serverURL, "https://", "wss://", 1)
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1) + "/api/v1/ws?access_token=" + url.QueryEscape(tokens.AccessToken)
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}
		for {
			_, data, readErr := conn.ReadMessage()
			err = readErr
			if err != nil {
				conn.Close()
				break
			}
			syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			var event struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(data, &event)
			if event.Type == "membership" {
				_ = c.bootstrap(syncCtx)
				c.emitStatus(syncCtx)
			} else {
				_ = c.SyncNow(syncCtx)
			}
			cancel()
		}
	}
}

func (c *SyncClient) ListRemoteMembers(ctx context.Context, notebookID string) ([]NotebookMember, error) {
	var members []NotebookMember
	if err := c.authenticatedRequest(ctx, http.MethodGet, "/api/v1/notebooks/"+url.PathEscape(notebookID)+"/members", nil, &members); err != nil {
		return nil, err
	}
	for i := range members {
		members[i].NotebookID = notebookID
	}
	return members, nil
}

func (c *SyncClient) CreateInvitation(ctx context.Context, input InvitationInput) (InvitationResult, error) {
	if input.Role != "editor" && input.Role != "viewer" {
		return InvitationResult{}, errors.New("成员权限无效")
	}
	var result InvitationResult
	err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/notebooks/"+url.PathEscape(input.NotebookID)+"/invites", map[string]string{"username": input.Username, "role": input.Role}, &result)
	return result, err
}

func (c *SyncClient) UpdateRemoteMember(ctx context.Context, input MemberUpdateInput) error {
	if input.Role != "editor" && input.Role != "viewer" {
		return errors.New("成员权限无效")
	}
	if err := c.authenticatedRequest(ctx, http.MethodPatch, "/api/v1/notebooks/"+url.PathEscape(input.NotebookID)+"/members/"+url.PathEscape(input.UserID), map[string]string{"role": input.Role}, nil); err != nil {
		return err
	}
	return c.bootstrap(ctx)
}

func (c *SyncClient) RemoveRemoteMember(ctx context.Context, notebookID, userID string) error {
	if err := c.authenticatedRequest(ctx, http.MethodDelete, "/api/v1/notebooks/"+url.PathEscape(notebookID)+"/members/"+url.PathEscape(userID), nil, nil); err != nil {
		return err
	}
	return c.bootstrap(ctx)
}

func (c *SyncClient) TransferRemoteOwnership(ctx context.Context, notebookID, userID string) error {
	if err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/notebooks/"+url.PathEscape(notebookID)+"/transfer", map[string]string{"userId": userID}, nil); err != nil {
		return err
	}
	return c.bootstrap(ctx)
}

func (c *SyncClient) AcceptInvitation(ctx context.Context, token string) error {
	var result map[string]string
	if err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/invitations/accept", map[string]string{"token": strings.TrimSpace(token)}, &result); err != nil {
		return err
	}
	return c.bootstrap(ctx)
}

func (c *SyncClient) AcquireRemoteLease(ctx context.Context, noteID, notebookID string, takeover bool) (EditLease, error) {
	var lease EditLease
	err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/notes/"+url.PathEscape(noteID)+"/lease", map[string]any{"notebookId": notebookID, "takeover": takeover}, &lease)
	return lease, err
}

func (c *SyncClient) ReleaseRemoteLease(ctx context.Context, noteID string) error {
	return c.authenticatedRequest(ctx, http.MethodDelete, "/api/v1/notes/"+url.PathEscape(noteID)+"/lease", nil, nil)
}

func upsertNotificationsTx(ctx context.Context, tx *sql.Tx, items []Notification) error {
	for _, item := range items {
		if item.ID == "" || item.EntityID == "" {
			continue
		}
		var readAt any
		if item.ReadAt != "" {
			readAt = item.ReadAt
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications(id,user_id,kind,entity_id,message,read_at,created_at) VALUES(?,'local',?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET kind=excluded.kind,entity_id=excluded.entity_id,message=excluded.message,
			read_at=COALESCE(excluded.read_at,notifications.read_at)`, item.ID, item.Kind, item.EntityID, item.Message, readAt,
			coalesce(item.CreatedAt, nowString())); err != nil {
			return err
		}
	}
	return nil
}

func (c *SyncClient) pullNotifications(ctx context.Context) error {
	var items []Notification
	if err := c.authenticatedRequest(ctx, http.MethodGet, "/api/v1/notifications", nil, &items); err != nil {
		return err
	}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertNotificationsTx(ctx, tx, items); err != nil {
		return err
	}
	return tx.Commit()
}

func (c *SyncClient) MarkNotificationRead(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("通知 ID 不能为空")
	}
	if err := c.authenticatedRequest(ctx, http.MethodPost, "/api/v1/notifications/"+url.PathEscape(id)+"/read", nil, nil); err != nil {
		return err
	}
	return c.store.MarkNotificationRead(ctx, id)
}

func verifyAttachmentFile(path, expectedHash string, expectedSize int64) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != expectedSize {
		return false
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), expectedHash)
}

func (c *SyncClient) EnsureAttachmentCached(ctx context.Context, id string) error {
	item, path, err := c.store.attachmentPath(ctx, id)
	if err != nil {
		return err
	}
	if verifyAttachmentFile(path, item.SHA256, item.Size) {
		return nil
	}
	if err := validateAttachment(item.FileName, item.MimeType, item.Size); err != nil {
		return err
	}
	var prepared struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := c.authenticatedRequest(ctx, http.MethodGet, "/api/v1/attachments/download?id="+url.QueryEscape(id), nil, &prepared); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, prepared.DownloadURL, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("附件下载失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("附件下载失败，存储服务返回 %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(response.Body, maxAttachmentBytes+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != item.Size || written > maxAttachmentBytes || !strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), item.SHA256) {
		_, _ = c.store.db.ExecContext(ctx, `UPDATE attachments SET status='failed',updated_at=? WHERE id=?`, nowString(), id)
		return errors.New("附件校验失败，文件未写入缓存")
	}
	_ = os.Remove(path)
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	_, err = c.store.db.ExecContext(ctx, `UPDATE attachments SET status='synced',updated_at=? WHERE id=?`, nowString(), id)
	return err
}

func (s *Store) ListNotifications(ctx context.Context) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,entity_id,message,COALESCE(read_at,''),created_at FROM notifications WHERE user_id=? ORDER BY read_at IS NULL DESC,created_at DESC LIMIT 100`, localUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Notification{}
	for rows.Next() {
		var item Notification
		if err := rows.Scan(&item.ID, &item.Kind, &item.EntityID, &item.Message, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Store) MarkNotificationRead(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at=? WHERE id=? AND user_id=?`, nowString(), id, localUserID)
	return err
}
