package syncserver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type contextKey string

const userContextKey contextKey = "todo-user"

type Server struct {
	config  Config
	db      *pgxpool.Pool
	objects *minio.Client
	hub     *hub
	logger  *slog.Logger
	mux     *http.ServeMux
}

func New(ctx context.Context, config Config, logger *slog.Logger) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	pool, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	objects, err := minio.New(config.MinIOEndpoint, &minio.Options{
		Creds: credentials.NewStaticV4(config.MinIOAccessKey, config.MinIOSecretKey, ""), Secure: config.MinIOSecure,
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	exists, err := objects.BucketExists(ctx, config.MinIOBucket)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect object storage: %w", err)
	}
	if !exists {
		if err := objects.MakeBucket(ctx, config.MinIOBucket, minio.MakeBucketOptions{}); err != nil {
			pool.Close()
			return nil, fmt.Errorf("create attachment bucket: %w", err)
		}
	}
	s := &Server{config: config, db: pool, objects: objects, hub: newHub(), logger: logger, mux: http.NewServeMux()}
	if err := s.cleanupRetention(ctx); err != nil {
		logger.Warn("retention cleanup failed", "error", err)
	}
	s.routes()
	return s, nil
}

func (s *Server) cleanupRetention(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT a.id,a.object_key FROM attachments a WHERE a.deleted_at<now()-interval '7 days'
		AND NOT EXISTS (SELECT 1 FROM entities e WHERE e.entity_type IN ('note','note_version') AND e.deleted=false
		AND COALESCE(e.payload->'attachmentIds','[]'::jsonb) ? a.id)`)
	if err != nil {
		return err
	}
	type orphan struct{ id, objectKey string }
	orphans := []orphan{}
	for rows.Next() {
		var item orphan
		if err := rows.Scan(&item.id, &item.objectKey); err != nil {
			rows.Close()
			return err
		}
		orphans = append(orphans, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range orphans {
		if err := s.objects.RemoveObject(ctx, s.config.MinIOBucket, item.objectKey, minio.RemoveObjectOptions{}); err != nil {
			continue
		}
		_, _ = s.db.Exec(ctx, `DELETE FROM attachments WHERE id=$1`, item.id)
	}
	statements := []string{
		`DELETE FROM refresh_tokens WHERE expires_at<now() OR revoked_at<now()-interval '30 days'`,
		`DELETE FROM invitations WHERE expires_at<now()-interval '7 days'`,
		`DELETE FROM comments WHERE deleted_at<now()-interval '90 days'`,
		`WITH ranked AS (SELECT entity_id,ROW_NUMBER() OVER(PARTITION BY payload->>'noteId' ORDER BY updated_at DESC) AS position
			FROM entities WHERE entity_type='note_version') DELETE FROM entities WHERE entity_type='note_version'
			AND (updated_at<now()-interval '90 days' OR entity_id IN (SELECT entity_id FROM ranked WHERE position>100))`,
		`DELETE FROM changes WHERE created_at<now()-interval '90 days'`,
		`DELETE FROM entities e USING tombstones t WHERE e.entity_type=t.entity_type AND e.entity_id=t.entity_id AND t.deleted_at<now()-interval '90 days'`,
		`DELETE FROM tombstones WHERE deleted_at<now()-interval '90 days'`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) Close() { s.db.Close() }

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/refresh", s.handleRefresh)
	s.mux.Handle("POST /api/v1/auth/logout", s.requireAuth(http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("POST /api/v1/invitations/accept", s.requireAuth(http.HandlerFunc(s.handleAcceptInvitation)))
	s.mux.Handle("POST /api/v1/sync/push", s.requireAuth(http.HandlerFunc(s.handleSyncPush)))
	s.mux.Handle("GET /api/v1/sync/pull", s.requireAuth(http.HandlerFunc(s.handleSyncPull)))
	s.mux.Handle("GET /api/v1/sync/bootstrap", s.requireAuth(http.HandlerFunc(s.handleSyncBootstrap)))
	s.mux.Handle("GET /api/v1/notifications", s.requireAuth(http.HandlerFunc(s.handleNotifications)))
	s.mux.Handle("POST /api/v1/notifications/", s.requireAuth(http.HandlerFunc(s.handleNotifications)))
	s.mux.Handle("/api/v1/notebooks/", s.requireAuth(http.HandlerFunc(s.handleNotebookRoute)))
	s.mux.Handle("/api/v1/notes/", s.requireAuth(http.HandlerFunc(s.handleNoteRoute)))
	s.mux.Handle("POST /api/v1/attachments/prepare", s.requireAuth(http.HandlerFunc(s.handleAttachmentPrepare)))
	s.mux.Handle("POST /api/v1/attachments/complete", s.requireAuth(http.HandlerFunc(s.handleAttachmentComplete)))
	s.mux.Handle("GET /api/v1/attachments/download", s.requireAuth(http.HandlerFunc(s.handleAttachmentDownload)))
	s.mux.HandleFunc("GET /api/v1/ws", s.handleWebSocket)
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.parseAccessToken(r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "登录已失效")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func requestUser(r *http.Request) authUser { return r.Context().Value(userContextKey).(authUser) }

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求数据无效")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": code, "message": message})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "1"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input struct{ Username, Password string }
	if !decodeJSON(w, r, &input, 16*1024) {
		return
	}
	user, err := s.authenticate(r.Context(), input.Username, input.Password)
	if err != nil {
		time.Sleep(250 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	access, refresh, err := s.issueTokens(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "无法创建登录会话")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accessToken": access, "refreshToken": refresh, "expiresIn": 900, "user": user})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &input, 16*1024) {
		return
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input.RefreshToken)))
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "database_error", "无法刷新会话")
		return
	}
	defer tx.Rollback(r.Context())
	var user authUser
	var tokenID string
	err = tx.QueryRow(r.Context(), `SELECT rt.id, u.id, u.username, u.is_admin FROM refresh_tokens rt JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1 AND rt.revoked_at IS NULL AND rt.expires_at > now() AND u.disabled = false FOR UPDATE`, hash).
		Scan(&tokenID, &user.ID, &user.Username, &user.Admin)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "刷新令牌无效")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`, tokenID); err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, 500, "database_error", "无法刷新会话")
		return
	}
	access, refresh, err := s.issueTokens(r.Context(), user)
	if err != nil {
		writeError(w, 500, "token_error", "无法刷新会话")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accessToken": access, "refreshToken": refresh, "expiresIn": 900})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &input, 16*1024) {
		return
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input.RefreshToken)))
	_, _ = s.db.Exec(r.Context(), `UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`, hash)
	w.WriteHeader(http.StatusNoContent)
}

type syncOperation struct {
	ID          string          `json:"id"`
	EntityType  string          `json:"entityType"`
	EntityID    string          `json:"entityId"`
	Action      string          `json:"action"`
	BaseVersion int64           `json:"baseVersion"`
	Payload     json.RawMessage `json:"payload"`
}

type syncResult struct {
	ID             string `json:"id"`
	EntityID       string `json:"entityId"`
	Version        int64  `json:"version"`
	ConflictCopyID string `json:"conflictCopyId,omitempty"`
}

func payloadNotebookID(payload json.RawMessage) string {
	var object map[string]any
	if json.Unmarshal(payload, &object) != nil {
		return ""
	}
	value, _ := object["notebookId"].(string)
	return value
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Operations []syncOperation `json:"operations"`
	}
	if !decodeJSON(w, r, &input, 4*1024*1024) {
		return
	}
	if len(input.Operations) > 500 {
		writeError(w, 400, "too_many_operations", "单次最多同步 500 个操作")
		return
	}
	user := requestUser(r)
	results := make([]syncResult, 0, len(input.Operations))
	for _, operation := range input.Operations {
		result, err := s.applyOperation(r.Context(), user, operation)
		if err != nil {
			writeError(w, http.StatusConflict, "sync_conflict", err.Error())
			return
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) applyOperation(ctx context.Context, user authUser, operation syncOperation) (syncResult, error) {
	if operation.ID == "" || operation.EntityID == "" || operation.EntityType == "" || !json.Valid(operation.Payload) {
		return syncResult{}, errors.New("同步操作无效")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return syncResult{}, err
	}
	defer tx.Rollback(ctx)
	var already bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM operations WHERE operation_id = $1)`, operation.ID).Scan(&already); err != nil {
		return syncResult{}, err
	}
	if already {
		var version int64
		_ = tx.QueryRow(ctx, `SELECT version FROM entities WHERE entity_type = $1 AND entity_id = $2`, operation.EntityType, operation.EntityID).Scan(&version)
		return syncResult{ID: operation.ID, EntityID: operation.EntityID, Version: version}, nil
	}
	notebookID := payloadNotebookID(operation.Payload)
	if operation.EntityType == "comment" && notebookID == "" {
		var body struct {
			NoteID string `json:"noteId"`
		}
		_ = json.Unmarshal(operation.Payload, &body)
		_ = tx.QueryRow(ctx, `SELECT notebook_id FROM entities WHERE entity_type = 'note' AND entity_id = $1 AND deleted = false`, body.NoteID).Scan(&notebookID)
		var object map[string]any
		if json.Unmarshal(operation.Payload, &object) == nil {
			object["authorId"], object["authorName"] = user.ID, user.Username
			operation.Payload, _ = json.Marshal(object)
		}
		if notebookID == "" {
			return syncResult{}, errors.New("评论关联的笔记不存在")
		}
	}
	if operation.EntityType == "note_version" {
		var body struct {
			NoteID string `json:"noteId"`
		}
		_ = json.Unmarshal(operation.Payload, &body)
		if body.NoteID == "" {
			return syncResult{}, errors.New("历史版本数据无效")
		}
		if notebookID == "" {
			_ = tx.QueryRow(ctx, `SELECT notebook_id FROM entities WHERE entity_type='note' AND entity_id=$1 AND deleted=false`, body.NoteID).Scan(&notebookID)
		}
		if notebookID == "" {
			return syncResult{}, errors.New("历史版本关联的笔记不存在")
		}
		var object map[string]any
		if json.Unmarshal(operation.Payload, &object) == nil {
			object["actorId"], object["actorName"] = user.ID, user.Username
			operation.Payload, _ = json.Marshal(object)
		}
	}
	if operation.EntityType == "notebook" {
		var body struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(operation.Payload, &body)
		var existingOwner string
		existingErr := tx.QueryRow(ctx, `SELECT owner_id FROM notebooks WHERE id=$1 FOR UPDATE`, operation.EntityID).Scan(&existingOwner)
		if existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
			return syncResult{}, existingErr
		}
		if existingErr == nil && existingOwner != user.ID {
			return syncResult{}, errors.New("只有所有者可以修改笔记本")
		}
		notebookID = operation.EntityID
		if operation.Action == "delete" {
			if existingErr != nil {
				return syncResult{}, errors.New("笔记本不存在")
			}
			if _, err = tx.Exec(ctx, `UPDATE notebooks SET deleted_at=now(),version=version+1,updated_at=now() WHERE id=$1`, operation.EntityID); err != nil {
				return syncResult{}, err
			}
		} else {
			if strings.TrimSpace(body.Name) == "" {
				body.Name = "我的笔记"
			}
			_, err = tx.Exec(ctx, `INSERT INTO notebooks(id, owner_id, name) VALUES ($1, $2, $3)
				ON CONFLICT(id) DO UPDATE SET name = excluded.name, version = notebooks.version + 1, updated_at = now(), deleted_at = NULL`, operation.EntityID, user.ID, body.Name)
			if err != nil {
				return syncResult{}, err
			}
			_, err = tx.Exec(ctx, `INSERT INTO memberships(notebook_id, user_id, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`, operation.EntityID, user.ID)
			if err != nil {
				return syncResult{}, err
			}
		}
	} else if notebookID != "" {
		if err := requireNotebookRoleTx(ctx, tx, notebookID, user.ID, true); err != nil {
			return syncResult{}, err
		}
	}
	var currentVersion int64
	var deleted bool
	err = tx.QueryRow(ctx, `SELECT version, deleted FROM entities WHERE entity_type = $1 AND entity_id = $2 FOR UPDATE`, operation.EntityType, operation.EntityID).Scan(&currentVersion, &deleted)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return syncResult{}, err
	}
	if deleted && operation.Action != "restore" && operation.Action != "delete" {
		return syncResult{}, errors.New("内容已经删除，请先恢复")
	}
	targetID := operation.EntityID
	conflictID := ""
	if operation.EntityType == "note" && currentVersion > 0 && operation.BaseVersion > 0 && operation.BaseVersion != currentVersion && operation.Action != "delete" {
		conflictID = uuid.NewString()
		targetID = conflictID
		currentVersion = 0
		var object map[string]any
		if json.Unmarshal(operation.Payload, &object) == nil {
			object["id"] = conflictID
			if title, ok := object["title"].(string); ok {
				object["title"] = title + " (冲突副本 - " + user.Username + ")"
			}
			operation.Payload, _ = json.Marshal(object)
		}
	}
	nextVersion := currentVersion + 1
	isDelete := operation.Action == "delete"
	_, err = tx.Exec(ctx, `INSERT INTO entities(entity_type, entity_id, owner_id, notebook_id, payload, version, deleted)
		VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7)
		ON CONFLICT(entity_type, entity_id) DO UPDATE SET payload = excluded.payload, version = excluded.version,
		deleted = excluded.deleted, notebook_id = excluded.notebook_id, updated_at = now()`, operation.EntityType, targetID, user.ID, notebookID, operation.Payload, nextVersion, isDelete)
	if err != nil {
		return syncResult{}, err
	}
	if isDelete {
		if _, err := tx.Exec(ctx, `INSERT INTO tombstones(entity_type,entity_id,owner_id,notebook_id,deleted_at) VALUES($1,$2,$3,NULLIF($4,''),now())
			ON CONFLICT(entity_type,entity_id) DO UPDATE SET owner_id=excluded.owner_id,notebook_id=excluded.notebook_id,deleted_at=excluded.deleted_at`, operation.EntityType, targetID, user.ID, notebookID); err != nil {
			return syncResult{}, err
		}
	} else if operation.Action == "restore" {
		if _, err := tx.Exec(ctx, `DELETE FROM tombstones WHERE entity_type=$1 AND entity_id=$2`, operation.EntityType, targetID); err != nil {
			return syncResult{}, err
		}
	}
	if operation.EntityType == "comment" {
		var body struct {
			NoteID   string `json:"noteId"`
			ParentID string `json:"parentId"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal(operation.Payload, &body); err != nil || body.NoteID == "" {
			return syncResult{}, errors.New("评论数据无效")
		}
		if isDelete {
			_, err = tx.Exec(ctx, `UPDATE comments SET deleted_at = now(), updated_at = now() WHERE id = $1 AND author_id = $2`, operation.EntityID, user.ID)
		} else {
			var parent any
			if body.ParentID != "" {
				var validParent bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE id=$1 AND note_id=$2 AND deleted_at IS NULL)`, body.ParentID, body.NoteID).Scan(&validParent); err != nil || !validParent {
					return syncResult{}, errors.New("回复的评论不存在")
				}
				parent = body.ParentID
			}
			_, err = tx.Exec(ctx, `INSERT INTO comments(id,note_id,notebook_id,parent_id,author_id,content) VALUES($1,$2,$3,$4,$5,$6)
				ON CONFLICT(id) DO UPDATE SET content=excluded.content,deleted_at=NULL,updated_at=now()`, operation.EntityID, body.NoteID, notebookID, parent, user.ID, strings.TrimSpace(body.Content))
		}
		if err != nil {
			return syncResult{}, err
		}
	}
	if operation.EntityType == "attachment" && isDelete {
		if _, err := tx.Exec(ctx, `UPDATE attachments SET deleted_at=now(),updated_at=now() WHERE id=$1 AND notebook_id=$2`, operation.EntityID, notebookID); err != nil {
			return syncResult{}, err
		}
	}
	var sequence int64
	err = tx.QueryRow(ctx, `INSERT INTO changes(owner_id, notebook_id, entity_type, entity_id, action, version, payload)
		VALUES ($1, NULLIF($2,''), $3, $4, $5, $6, $7) RETURNING sequence`, user.ID, notebookID, operation.EntityType, targetID, operation.Action, nextVersion, operation.Payload).Scan(&sequence)
	if err != nil {
		return syncResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO operations(operation_id, user_id) VALUES ($1, $2)`, operation.ID, user.ID); err != nil {
		return syncResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return syncResult{}, err
	}
	if operation.EntityType == "comment" && !isDelete {
		var body struct{ NoteID, Content string }
		_ = json.Unmarshal(operation.Payload, &body)
		s.createMentionNotifications(ctx, notebookID, body.NoteID, user, body.Content)
	}
	s.hub.notifyUser(user.ID, realtimeMessage{Type: "change", EntityType: operation.EntityType, EntityID: targetID, NotebookID: notebookID, Sequence: sequence})
	s.notifyNotebookMembers(ctx, notebookID, user.ID, realtimeMessage{Type: "change", EntityType: operation.EntityType, EntityID: targetID, NotebookID: notebookID, Sequence: sequence})
	return syncResult{ID: operation.ID, EntityID: targetID, Version: nextVersion, ConflictCopyID: conflictID}, nil
}

func requireNotebookRoleTx(ctx context.Context, tx pgx.Tx, notebookID, userID string, edit bool) error {
	var role string
	if err := tx.QueryRow(ctx, `SELECT role FROM memberships m JOIN notebooks n ON n.id = m.notebook_id WHERE m.notebook_id = $1 AND m.user_id = $2 AND n.deleted_at IS NULL`, notebookID, userID).Scan(&role); err != nil {
		return errors.New("没有访问这个笔记本的权限")
	}
	if edit && role == "viewer" {
		return errors.New("这个笔记本为只读")
	}
	return nil
}

func (s *Server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		cursor = "0"
	}
	var requested, minimum int64
	if _, err := fmt.Sscan(cursor, &requested); err != nil {
		writeError(w, 400, "invalid_cursor", "同步游标无效")
		return
	}
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(MIN(sequence),0) FROM changes`).Scan(&minimum)
	if requested > 0 && minimum > requested+1 {
		writeError(w, 409, "full_resync_required", "设备离线时间过长，需要完整重新同步")
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 500)
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.Query(r.Context(), `SELECT c.sequence, c.entity_type, c.entity_id, c.action, c.version, c.payload, c.created_at
		FROM changes c WHERE c.sequence > $1 AND (c.owner_id = $2 OR c.notebook_id IN (SELECT notebook_id FROM memberships WHERE user_id = $2))
		ORDER BY c.sequence LIMIT $3`, cursor, user.ID, limit)
	if err != nil {
		writeError(w, 500, "database_error", "无法读取同步变更")
		return
	}
	defer rows.Close()
	changes := []map[string]any{}
	lastCursor := cursor
	for rows.Next() {
		var sequence, version int64
		var entityType, entityID, action string
		var payload json.RawMessage
		var created time.Time
		if err := rows.Scan(&sequence, &entityType, &entityID, &action, &version, &payload, &created); err != nil {
			writeError(w, 500, "database_error", "无法读取同步变更")
			return
		}
		lastCursor = fmt.Sprint(sequence)
		changes = append(changes, map[string]any{"sequence": sequence, "entityType": entityType, "entityId": entityID, "action": action, "version": version, "payload": payload, "createdAt": created})
	}
	writeJSON(w, 200, map[string]any{"cursor": lastCursor, "changes": changes, "hasMore": len(changes) == limit})
}

func (s *Server) handleSyncBootstrap(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	tx, err := s.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "database_error", "无法读取同步资料")
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), `SELECT n.id, n.name, n.owner_id, m.role, n.version, n.created_at, n.updated_at,
		EXISTS(SELECT 1 FROM memberships other WHERE other.notebook_id=n.id AND other.user_id<>n.owner_id)
		FROM notebooks n JOIN memberships m ON m.notebook_id = n.id WHERE m.user_id = $1 AND n.deleted_at IS NULL ORDER BY n.created_at`, user.ID)
	if err != nil {
		writeError(w, 500, "database_error", "无法读取同步资料")
		return
	}
	defer rows.Close()
	notebooks := []map[string]any{}
	for rows.Next() {
		var id, name, ownerID, role string
		var version int64
		var created, updated time.Time
		var shared bool
		if rows.Scan(&id, &name, &ownerID, &role, &version, &created, &updated, &shared) == nil {
			kind := "personal"
			if shared || ownerID != user.ID {
				kind = "shared"
			}
			notebooks = append(notebooks, map[string]any{"id": id, "name": name, "kind": kind, "ownerId": ownerID, "role": role, "serverVersion": version, "createdAt": created, "updatedAt": updated})
		}
	}
	rows.Close()
	membershipRows, err := tx.Query(r.Context(), `SELECT m.notebook_id,u.id,u.username,m.role,m.created_at
		FROM memberships m JOIN users u ON u.id=m.user_id
		WHERE m.notebook_id IN (SELECT notebook_id FROM memberships WHERE user_id=$1)
		ORDER BY m.notebook_id, CASE m.role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, u.username`, user.ID)
	if err != nil {
		writeError(w, 500, "database_error", "无法读取成员资料")
		return
	}
	memberships := []map[string]any{}
	for membershipRows.Next() {
		var notebookID, userID, username, role string
		var created time.Time
		if membershipRows.Scan(&notebookID, &userID, &username, &role, &created) == nil {
			memberships = append(memberships, map[string]any{"notebookId": notebookID, "userId": userID, "username": username, "role": role, "createdAt": created})
		}
	}
	membershipRows.Close()
	entityRows, err := tx.Query(r.Context(), `SELECT entity_type,entity_id,CASE WHEN deleted THEN 'delete' ELSE 'upsert' END,version,payload
		FROM entities WHERE owner_id=$1 OR notebook_id IN (SELECT notebook_id FROM memberships WHERE user_id=$1)
		ORDER BY CASE entity_type WHEN 'notebook' THEN 0 WHEN 'list' THEN 1 WHEN 'tag' THEN 2 WHEN 'task' THEN 3
		WHEN 'note_tag' THEN 4 WHEN 'attachment' THEN 5 WHEN 'note' THEN 6 WHEN 'comment' THEN 7 ELSE 8 END, updated_at`, user.ID)
	if err != nil {
		writeError(w, 500, "database_error", "无法读取同步内容")
		return
	}
	entities := []map[string]any{}
	for entityRows.Next() {
		var entityType, entityID, action string
		var version int64
		var payload json.RawMessage
		if entityRows.Scan(&entityType, &entityID, &action, &version, &payload) == nil {
			entities = append(entities, map[string]any{"entityType": entityType, "entityId": entityID, "action": action, "version": version, "payload": payload})
		}
	}
	entityRows.Close()
	notificationRows, err := tx.Query(r.Context(), `SELECT id,kind,entity_id,message,read_at,created_at FROM notifications
		WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, user.ID)
	if err != nil {
		writeError(w, 500, "database_error", "无法读取通知")
		return
	}
	notifications := []map[string]any{}
	for notificationRows.Next() {
		var id, kind, entityID, message string
		var readAt *time.Time
		var created time.Time
		if notificationRows.Scan(&id, &kind, &entityID, &message, &readAt, &created) == nil {
			notifications = append(notifications, map[string]any{"id": id, "kind": kind, "entityId": entityID, "message": message, "readAt": readAt, "createdAt": created})
		}
	}
	notificationRows.Close()
	var cursor, unread int64
	_ = tx.QueryRow(r.Context(), `SELECT COALESCE(MAX(sequence),0) FROM changes`).Scan(&cursor)
	_ = tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, user.ID).Scan(&unread)
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "database_error", "无法读取同步资料")
		return
	}
	writeJSON(w, 200, map[string]any{"user": user, "notebooks": notebooks, "memberships": memberships, "entities": entities, "notifications": notifications, "cursor": cursor, "unreadCount": unread})
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/notifications" {
		rows, err := s.db.Query(r.Context(), `SELECT id,kind,entity_id,message,read_at,created_at FROM notifications
			WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, user.ID)
		if err != nil {
			writeError(w, 500, "database_error", "无法读取通知")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, kind, entityID, message string
			var readAt *time.Time
			var created time.Time
			if rows.Scan(&id, &kind, &entityID, &message, &readAt, &created) == nil {
				items = append(items, map[string]any{"id": id, "kind": kind, "entityId": entityID, "message": message, "readAt": readAt, "createdAt": created})
			}
		}
		writeJSON(w, 200, items)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/notifications/"), "/"), "/")
	if r.Method != http.MethodPost || len(parts) != 2 || parts[1] != "read" || parts[0] == "" {
		writeError(w, 404, "not_found", "接口不存在")
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND user_id=$2`, parts[0], user.ID)
	if err != nil {
		writeError(w, 500, "database_error", "无法更新通知")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "通知不存在")
		return
	}
	writeJSON(w, 200, map[string]any{"id": parts[0], "read": true})
}

func (s *Server) handleNotebookRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/notebooks/"), "/"), "/")
	if len(parts) < 2 {
		writeError(w, 404, "not_found", "接口不存在")
		return
	}
	notebookID, action := parts[0], parts[1]
	user := requestUser(r)
	if action == "members" {
		if r.Method == http.MethodGet && len(parts) == 2 {
			if !s.requireNotebookRole(r.Context(), notebookID, user.ID, false, w) {
				return
			}
			rows, err := s.db.Query(r.Context(), `SELECT u.id, u.username, m.role, m.created_at FROM memberships m JOIN users u ON u.id = m.user_id WHERE m.notebook_id = $1 ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, u.username`, notebookID)
			if err != nil {
				writeError(w, 500, "database_error", "无法读取成员")
				return
			}
			defer rows.Close()
			members := []map[string]any{}
			for rows.Next() {
				var id, username, role string
				var created time.Time
				if rows.Scan(&id, &username, &role, &created) == nil {
					members = append(members, map[string]any{"userId": id, "username": username, "role": role, "createdAt": created})
				}
			}
			writeJSON(w, 200, members)
			return
		}
		if len(parts) == 3 && (r.Method == http.MethodPatch || r.Method == http.MethodDelete) {
			targetID := parts[2]
			var actorRole, targetRole string
			_ = s.db.QueryRow(r.Context(), `SELECT role FROM memberships WHERE notebook_id=$1 AND user_id=$2`, notebookID, user.ID).Scan(&actorRole)
			if err := s.db.QueryRow(r.Context(), `SELECT role FROM memberships WHERE notebook_id=$1 AND user_id=$2`, notebookID, targetID).Scan(&targetRole); err != nil {
				writeError(w, 404, "member_not_found", "成员不存在")
				return
			}
			if r.Method == http.MethodPatch {
				if actorRole != "owner" || targetRole == "owner" {
					writeError(w, 403, "owner_required", "只有所有者可以修改成员权限")
					return
				}
				var input struct {
					Role string `json:"role"`
				}
				if !decodeJSON(w, r, &input, 8*1024) {
					return
				}
				if input.Role != "editor" && input.Role != "viewer" {
					writeError(w, 400, "invalid_role", "成员权限无效")
					return
				}
				if _, err := s.db.Exec(r.Context(), `UPDATE memberships SET role=$1 WHERE notebook_id=$2 AND user_id=$3`, input.Role, notebookID, targetID); err != nil {
					writeError(w, 500, "database_error", "无法更新成员权限")
					return
				}
			} else {
				leaving := targetID == user.ID
				if targetRole == "owner" {
					writeError(w, 409, "transfer_required", "所有者必须先转让所有权")
					return
				}
				if !leaving && actorRole != "owner" {
					writeError(w, 403, "owner_required", "只有所有者可以移除成员")
					return
				}
				if _, err := s.db.Exec(r.Context(), `DELETE FROM memberships WHERE notebook_id=$1 AND user_id=$2`, notebookID, targetID); err != nil {
					writeError(w, 500, "database_error", "无法移除成员")
					return
				}
			}
			message := realtimeMessage{Type: "membership", EntityType: "membership", EntityID: targetID, NotebookID: notebookID}
			s.hub.notifyUser(targetID, message)
			s.notifyNotebookMembers(r.Context(), notebookID, "", message)
			writeJSON(w, 200, map[string]any{"notebookId": notebookID, "userId": targetID, "updated": true})
			return
		}
		writeError(w, 405, "method_not_allowed", "请求方法不支持")
		return
	}
	if action == "transfer" && r.Method == http.MethodPost {
		var input struct {
			UserID string `json:"userId"`
		}
		if !decodeJSON(w, r, &input, 8*1024) {
			return
		}
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			writeError(w, 500, "database_error", "无法转让所有权")
			return
		}
		defer tx.Rollback(r.Context())
		var actorRole, targetRole string
		_ = tx.QueryRow(r.Context(), `SELECT role FROM memberships WHERE notebook_id=$1 AND user_id=$2 FOR UPDATE`, notebookID, user.ID).Scan(&actorRole)
		_ = tx.QueryRow(r.Context(), `SELECT role FROM memberships WHERE notebook_id=$1 AND user_id=$2 FOR UPDATE`, notebookID, input.UserID).Scan(&targetRole)
		if actorRole != "owner" || input.UserID == user.ID || targetRole == "" {
			writeError(w, 403, "invalid_transfer", "所有权只能转让给其他现有成员")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE memberships SET role=CASE WHEN user_id=$1 THEN 'owner' WHEN user_id=$2 THEN 'editor' ELSE role END WHERE notebook_id=$3`, input.UserID, user.ID, notebookID); err != nil {
			writeError(w, 500, "database_error", "无法转让所有权")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE notebooks SET owner_id=$1,version=version+1,updated_at=now() WHERE id=$2`, input.UserID, notebookID); err != nil || tx.Commit(r.Context()) != nil {
			writeError(w, 500, "database_error", "无法转让所有权")
			return
		}
		message := realtimeMessage{Type: "membership", EntityType: "membership", EntityID: input.UserID, NotebookID: notebookID}
		s.notifyNotebookMembers(r.Context(), notebookID, "", message)
		writeJSON(w, 200, map[string]any{"notebookId": notebookID, "ownerId": input.UserID})
		return
	}
	if action == "invites" && r.Method == http.MethodPost {
		if !s.requireNotebookRole(r.Context(), notebookID, user.ID, true, w) {
			return
		}
		var owner bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM memberships WHERE notebook_id=$1 AND user_id=$2 AND role='owner')`, notebookID, user.ID).Scan(&owner)
		if !owner {
			writeError(w, 403, "owner_required", "只有所有者可以邀请成员")
			return
		}
		var input struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		if !decodeJSON(w, r, &input, 16*1024) {
			return
		}
		if input.Role != "editor" && input.Role != "viewer" {
			writeError(w, 400, "invalid_role", "成员权限无效")
			return
		}
		token := uuid.NewString() + uuid.NewString()
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
		id := uuid.NewString()
		_, err := s.db.Exec(r.Context(), `INSERT INTO invitations(id,notebook_id,created_by,target_username,role,token_hash,expires_at) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,now()+interval '7 days')`, id, notebookID, user.ID, strings.ToLower(strings.TrimSpace(input.Username)), input.Role, hash)
		if err != nil {
			writeError(w, 500, "database_error", "无法创建邀请")
			return
		}
		writeJSON(w, 201, map[string]any{"id": id, "token": token, "expiresIn": 604800})
		return
	}
	writeError(w, 404, "not_found", "接口不存在")
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &input, 16*1024) {
		return
	}
	user := requestUser(r)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input.Token)))
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "database_error", "无法接受邀请")
		return
	}
	defer tx.Rollback(r.Context())
	var id, notebookID, role string
	var target *string
	err = tx.QueryRow(r.Context(), `SELECT id,notebook_id,role,target_username FROM invitations WHERE token_hash=$1 AND accepted_at IS NULL AND expires_at>now() FOR UPDATE`, hash).Scan(&id, &notebookID, &role, &target)
	if err != nil || target != nil && *target != "" && *target != user.Username {
		writeError(w, 403, "invalid_invitation", "邀请无效或已过期")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO memberships(notebook_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT(notebook_id,user_id) DO UPDATE SET role=excluded.role`, notebookID, user.ID, role); err != nil {
		writeError(w, 500, "database_error", "无法接受邀请")
		return
	}
	_, _ = tx.Exec(r.Context(), `UPDATE invitations SET accepted_at=now() WHERE id=$1`, id)
	if tx.Commit(r.Context()) != nil {
		writeError(w, 500, "database_error", "无法接受邀请")
		return
	}
	writeJSON(w, 200, map[string]string{"notebookId": notebookID, "role": role})
}

func (s *Server) requireNotebookRole(ctx context.Context, notebookID, userID string, edit bool, w http.ResponseWriter) bool {
	var role string
	err := s.db.QueryRow(ctx, `SELECT role FROM memberships m JOIN notebooks n ON n.id=m.notebook_id WHERE m.notebook_id=$1 AND m.user_id=$2 AND n.deleted_at IS NULL`, notebookID, userID).Scan(&role)
	if err != nil || edit && role == "viewer" {
		writeError(w, 403, "forbidden", "没有访问这个笔记本的权限")
		return false
	}
	return true
}

func (s *Server) noteNotebookID(ctx context.Context, noteID string) (string, error) {
	var notebookID string
	err := s.db.QueryRow(ctx, `SELECT notebook_id FROM entities WHERE entity_type='note' AND entity_id=$1 AND deleted=false`, noteID).Scan(&notebookID)
	return notebookID, err
}

func (s *Server) handleNoteRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/notes/"), "/"), "/")
	if len(parts) < 2 {
		writeError(w, 404, "not_found", "接口不存在")
		return
	}
	noteID, action := parts[0], parts[1]
	user := requestUser(r)
	if action == "lease" {
		s.handleLease(w, r, user, noteID)
		return
	}
	if action == "comments" {
		s.handleComments(w, r, user, noteID)
		return
	}
	if action == "versions" && r.Method == http.MethodGet {
		notebookID, err := s.noteNotebookID(r.Context(), noteID)
		if err != nil || !s.requireNotebookRole(r.Context(), notebookID, user.ID, false, w) {
			return
		}
		rows, err := s.db.Query(r.Context(), `SELECT payload,version,updated_at FROM entities WHERE entity_type='note_version' AND deleted=false AND payload->>'noteId'=$1 ORDER BY updated_at DESC LIMIT 100`, noteID)
		if err != nil {
			writeError(w, 500, "database_error", "无法读取历史")
			return
		}
		defer rows.Close()
		items := []json.RawMessage{}
		for rows.Next() {
			var payload json.RawMessage
			var version int64
			var updated time.Time
			if rows.Scan(&payload, &version, &updated) == nil {
				items = append(items, payload)
			}
		}
		writeJSON(w, 200, items)
		return
	}
	writeError(w, 404, "not_found", "接口不存在")
}

func (s *Server) handleLease(w http.ResponseWriter, r *http.Request, user authUser, noteID string) {
	var input struct {
		NotebookID string `json:"notebookId"`
		Takeover   bool   `json:"takeover"`
	}
	if r.Method == http.MethodDelete {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM edit_leases WHERE note_id=$1 AND holder_id=$2`, noteID, user.ID)
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost || !decodeJSON(w, r, &input, 16*1024) {
		return
	}
	notebookID, err := s.noteNotebookID(r.Context(), noteID)
	if err != nil || notebookID != input.NotebookID || !s.requireNotebookRole(r.Context(), notebookID, user.ID, true, w) {
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "database_error", "无法获取编辑权")
		return
	}
	defer tx.Rollback(r.Context())
	var holderID, holderName string
	var expires time.Time
	err = tx.QueryRow(r.Context(), `SELECT l.holder_id,u.username,l.expires_at FROM edit_leases l JOIN users u ON u.id=l.holder_id WHERE l.note_id=$1 FOR UPDATE`, noteID).Scan(&holderID, &holderName, &expires)
	if err == nil && holderID != user.ID && expires.After(time.Now()) && !input.Takeover {
		writeJSON(w, 200, map[string]any{"noteId": noteID, "holderId": holderID, "holderName": holderName, "expiresAt": expires, "editable": false})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO edit_leases(note_id,notebook_id,holder_id,expires_at) VALUES($1,$2,$3,now()+interval '2 minutes') ON CONFLICT(note_id) DO UPDATE SET notebook_id=excluded.notebook_id,holder_id=excluded.holder_id,expires_at=excluded.expires_at,updated_at=now()`, noteID, input.NotebookID, user.ID)
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, 500, "database_error", "无法获取编辑权")
		return
	}
	writeJSON(w, 200, map[string]any{"noteId": noteID, "holderId": user.ID, "holderName": user.Username, "expiresAt": time.Now().Add(2 * time.Minute), "editable": true})
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request, user authUser, noteID string) {
	notebookID, noteErr := s.noteNotebookID(r.Context(), noteID)
	if noteErr != nil {
		writeError(w, 404, "note_not_found", "笔记不存在")
		return
	}
	if r.Method == http.MethodGet {
		if r.URL.Query().Get("notebookId") != notebookID || !s.requireNotebookRole(r.Context(), notebookID, user.ID, false, w) {
			return
		}
		rows, err := s.db.Query(r.Context(), `SELECT c.id,c.parent_id,c.author_id,u.username,c.content,c.deleted_at,c.created_at,c.updated_at FROM comments c JOIN users u ON u.id=c.author_id WHERE c.note_id=$1 ORDER BY c.created_at`, noteID)
		if err != nil {
			writeError(w, 500, "database_error", "无法读取评论")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, authorID, username, content string
			var parent *string
			var deleted *time.Time
			var created, updated time.Time
			if rows.Scan(&id, &parent, &authorID, &username, &content, &deleted, &created, &updated) == nil {
				items = append(items, map[string]any{"id": id, "noteId": noteID, "parentId": parent, "authorId": authorID, "authorName": username, "content": content, "deletedAt": deleted, "createdAt": created, "updatedAt": updated})
			}
		}
		writeJSON(w, 200, items)
		return
	}
	if r.Method == http.MethodPost {
		var input struct{ NotebookID, ParentID, Content string }
		if !decodeJSON(w, r, &input, 64*1024) {
			return
		}
		if input.NotebookID != notebookID || !s.requireNotebookRole(r.Context(), notebookID, user.ID, true, w) {
			return
		}
		input.Content = strings.TrimSpace(input.Content)
		if input.Content == "" || len([]rune(input.Content)) > 4000 {
			writeError(w, 400, "invalid_comment", "评论内容无效")
			return
		}
		id := uuid.NewString()
		var parent any
		if input.ParentID != "" {
			var validParent bool
			if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM comments WHERE id=$1 AND note_id=$2 AND deleted_at IS NULL)`, input.ParentID, noteID).Scan(&validParent); err != nil || !validParent {
				writeError(w, 400, "invalid_parent", "回复的评论不存在")
				return
			}
			parent = input.ParentID
		}
		_, err := s.db.Exec(r.Context(), `INSERT INTO comments(id,note_id,notebook_id,parent_id,author_id,content) VALUES($1,$2,$3,$4,$5,$6)`, id, noteID, notebookID, parent, user.ID, input.Content)
		if err != nil {
			writeError(w, 500, "database_error", "无法添加评论")
			return
		}
		s.createMentionNotifications(r.Context(), notebookID, noteID, user, input.Content)
		s.notifyNotebookMembers(r.Context(), notebookID, user.ID, realtimeMessage{Type: "comment", EntityID: noteID, NotebookID: notebookID})
		writeJSON(w, 201, map[string]any{"id": id, "noteId": noteID, "authorId": user.ID, "authorName": user.Username, "content": input.Content, "createdAt": time.Now()})
		return
	}
	writeError(w, 405, "method_not_allowed", "请求方法不支持")
}

func (s *Server) createMentionNotifications(ctx context.Context, notebookID, noteID string, author authUser, content string) {
	for _, word := range strings.Fields(content) {
		if !strings.HasPrefix(word, "@") {
			continue
		}
		username := strings.Trim(strings.TrimPrefix(word, "@"), ",.，。:：;；")
		var userID string
		if s.db.QueryRow(ctx, `SELECT u.id FROM users u JOIN memberships m ON m.user_id=u.id WHERE u.username=$1 AND m.notebook_id=$2`, strings.ToLower(username), notebookID).Scan(&userID) == nil && userID != author.ID {
			message := author.Username + " 在评论中提到了你"
			_, _ = s.db.Exec(ctx, `INSERT INTO notifications(id,user_id,kind,entity_id,message) VALUES($1,$2,'mention',$3,$4)`, uuid.NewString(), userID, noteID, message)
			s.hub.notifyUser(userID, realtimeMessage{Type: "notification", EntityID: noteID, NotebookID: notebookID})
		}
	}
}

func (s *Server) handleAttachmentPrepare(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	var input struct {
		ID, NotebookID, FileName, MimeType, SHA256 string
		Size                                       int64
	}
	if !decodeJSON(w, r, &input, 64*1024) {
		return
	}
	if !s.requireNotebookRole(r.Context(), input.NotebookID, user.ID, true, w) {
		return
	}
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Size <= 0 || input.Size > s.config.MaxAttachmentMB*1024*1024 || !validServerAttachment(input.FileName, input.MimeType) {
		writeError(w, 400, "invalid_attachment", "附件类型或大小无效")
		return
	}
	objectKey := input.NotebookID + "/" + input.ID + "/" + url.PathEscape(filepath.Base(input.FileName))
	_, err := s.db.Exec(r.Context(), `INSERT INTO attachments(id,notebook_id,uploader_id,file_name,mime_type,size_bytes,sha256,object_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO UPDATE SET updated_at=now()`, input.ID, input.NotebookID, user.ID, filepath.Base(input.FileName), input.MimeType, input.Size, input.SHA256, objectKey)
	if err != nil {
		writeError(w, 500, "database_error", "无法准备附件上传")
		return
	}
	presigned, err := s.objects.PresignedPutObject(r.Context(), s.config.MinIOBucket, objectKey, 15*time.Minute)
	if err != nil {
		writeError(w, 500, "storage_error", "无法准备附件上传")
		return
	}
	writeJSON(w, 200, map[string]any{"attachmentId": input.ID, "uploadUrl": presigned.String(), "expiresIn": 900})
}
func validServerAttachment(name, mimeType string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true, ".txt": true, ".md": true, ".csv": true,
		".json": true, ".xml": true, ".yaml": true, ".yml": true, ".zip": true,
		".7z": true, ".rar": true,
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return allowed[ext] && !strings.Contains(mimeType, "svg") && !strings.Contains(mimeType, "html") &&
		!strings.Contains(mimeType, "javascript") && !strings.Contains(mimeType, "x-msdownload")
}
func (s *Server) handleAttachmentComplete(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	var input struct {
		AttachmentID string `json:"attachmentId"`
	}
	if !decodeJSON(w, r, &input, 16*1024) {
		return
	}
	var notebookID, objectKey, expectedSHA string
	var expected int64
	err := s.db.QueryRow(r.Context(), `SELECT notebook_id,object_key,size_bytes,sha256 FROM attachments WHERE id=$1 AND deleted_at IS NULL`, input.AttachmentID).Scan(&notebookID, &objectKey, &expected, &expectedSHA)
	if err != nil || !s.requireNotebookRole(r.Context(), notebookID, user.ID, true, w) {
		return
	}
	info, err := s.objects.StatObject(r.Context(), s.config.MinIOBucket, objectKey, minio.StatObjectOptions{})
	if err != nil || info.Size != expected {
		writeError(w, 400, "upload_incomplete", "附件上传未完成或大小不一致")
		return
	}
	object, err := s.objects.GetObject(r.Context(), s.config.MinIOBucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		writeError(w, 500, "storage_error", "无法校验附件")
		return
	}
	hash := sha256.New()
	written, hashErr := io.Copy(hash, io.LimitReader(object, expected+1))
	closeErr := object.Close()
	if hashErr != nil || closeErr != nil || written != expected || !strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), expectedSHA) {
		_ = s.objects.RemoveObject(r.Context(), s.config.MinIOBucket, objectKey, minio.RemoveObjectOptions{})
		writeError(w, 400, "checksum_mismatch", "附件校验失败")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE attachments SET completed=true,updated_at=now() WHERE id=$1`, input.AttachmentID)
	writeJSON(w, 200, map[string]any{"attachmentId": input.AttachmentID, "completed": true})
}
func (s *Server) handleAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	id := r.URL.Query().Get("id")
	var notebookID, objectKey string
	err := s.db.QueryRow(r.Context(), `SELECT notebook_id,object_key FROM attachments WHERE id=$1 AND completed=true AND deleted_at IS NULL`, id).Scan(&notebookID, &objectKey)
	if err != nil || !s.requireNotebookRole(r.Context(), notebookID, user.ID, false, w) {
		return
	}
	presigned, err := s.objects.PresignedGetObject(r.Context(), s.config.MinIOBucket, objectKey, 15*time.Minute, nil)
	if err != nil {
		writeError(w, 500, "storage_error", "无法创建下载链接")
		return
	}
	writeJSON(w, 200, map[string]any{"downloadUrl": presigned.String(), "expiresIn": 900})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("access_token")
	user, err := s.parseAccessToken("Bearer " + token)
	if err != nil {
		writeError(w, 401, "unauthorized", "登录已失效")
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, ReadBufferSize: 1024, WriteBufferSize: 1024}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &realtimeClient{userID: user.ID, conn: conn, send: make(chan []byte, 32)}
	s.hub.add(client)
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	go client.writePump(func() { s.hub.remove(client) })
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *Server) notifyNotebookMembers(ctx context.Context, notebookID, exceptUser string, message realtimeMessage) {
	if notebookID == "" {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT user_id FROM memberships WHERE notebook_id=$1 AND user_id<>$2`, notebookID, exceptUser)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			s.hub.notifyUser(id, message)
		}
	}
}
