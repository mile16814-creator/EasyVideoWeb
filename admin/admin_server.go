package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func generateRandomID(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

type mysqlConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Database string `json:"database"`
}

var db *sql.DB
var mysqlCfg mysqlConfig

func loadMySQLConfig(filePath string) error {
	mysqlCfg = mysqlConfig{
		Host:     "127.0.0.1",
		Port:     "3306",
		User:     "root",
		Pass:     "",
		Database: "boke",
	}
	var data []byte
	var err error
	for _, p := range []string{filePath, filepath.Join("..", filePath)} {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var cfg mysqlConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Host) != "" {
		mysqlCfg.Host = cfg.Host
	}
	if strings.TrimSpace(cfg.Port) != "" {
		mysqlCfg.Port = cfg.Port
	}
	if strings.TrimSpace(cfg.User) != "" {
		mysqlCfg.User = cfg.User
	}
	if strings.TrimSpace(cfg.Pass) != "" {
		if cfg.Pass != "your_mysql_password" {
			mysqlCfg.Pass = cfg.Pass
		}
	}
	if strings.TrimSpace(cfg.Database) != "" {
		mysqlCfg.Database = cfg.Database
	}
	if env := strings.TrimSpace(os.Getenv("MYSQL_HOST")); env != "" {
		mysqlCfg.Host = env
	}
	if env := strings.TrimSpace(os.Getenv("MYSQL_PORT")); env != "" {
		mysqlCfg.Port = env
	}
	if env := strings.TrimSpace(os.Getenv("MYSQL_USER")); env != "" {
		mysqlCfg.User = env
	}
	if env := strings.TrimSpace(os.Getenv("MYSQL_PASS")); env != "" {
		mysqlCfg.Pass = env
	}
	if env := strings.TrimSpace(os.Getenv("MYSQL_PASSWORD")); env != "" {
		mysqlCfg.Pass = env
	}
	if env := strings.TrimSpace(os.Getenv("MYSQL_DATABASE")); env != "" {
		mysqlCfg.Database = env
	}
	if strings.TrimSpace(mysqlCfg.Pass) == "" {
		return errors.New("mysql pass missing in mysql.local.json")
	}
	return nil
}

func initMySQL() {
	cfg := mysql.NewConfig()
	cfg.User = mysqlCfg.User
	cfg.Passwd = mysqlCfg.Pass
	cfg.Net = "tcp"
	cfg.Addr = mysqlCfg.Host + ":" + mysqlCfg.Port
	cfg.DBName = mysqlCfg.Database
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.Params = map[string]string{
		"charset": "utf8mb4",
	}
	dsn := cfg.FormatDSN()
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_categories (
		id VARCHAR(100) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		created_at DATETIME NOT NULL
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_categories (
		id VARCHAR(100) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		created_at DATETIME NOT NULL
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_library (
		video_id VARCHAR(255) PRIMARY KEY,
		category VARCHAR(100) NOT NULL,
		title VARCHAR(255) NOT NULL,
		description TEXT,
		tags TEXT,
		filename VARCHAR(255) NOT NULL,
		file_path VARCHAR(500) NOT NULL,
		thumb_url VARCHAR(500) NOT NULL,
		play_url VARCHAR(500) NOT NULL,
		duration_sec DOUBLE NOT NULL DEFAULT 0,
		size_bytes BIGINT NOT NULL DEFAULT 0,
		format_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		author_email VARCHAR(255) NOT NULL DEFAULT '',
		author_nickname VARCHAR(100) NOT NULL DEFAULT '',
		INDEX idx_video_library_category (category),
		INDEX idx_video_library_created_at (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE video_library ADD COLUMN review_status VARCHAR(20) NOT NULL DEFAULT 'approved'`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_review_messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		video_id VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		title VARCHAR(255) NOT NULL DEFAULT '',
		result VARCHAR(20) NOT NULL,
		reason TEXT,
		created_at DATETIME NOT NULL,
		INDEX idx_vrm_email (email),
		INDEX idx_vrm_created (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS manual_video_reviews (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		video_id VARCHAR(255) NOT NULL,
		requester_email VARCHAR(255) NOT NULL,
		title VARCHAR(255) NOT NULL DEFAULT '',
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		reviewer VARCHAR(100) NOT NULL DEFAULT '',
		review_note TEXT,
		reviewed_at DATETIME NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_mvr_status_created (status, created_at),
		INDEX idx_mvr_video (video_id),
		INDEX idx_mvr_requester (requester_email)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS manual_review_access_tokens (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		review_id BIGINT NOT NULL,
		video_id VARCHAR(255) NOT NULL,
		token VARCHAR(80) NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE KEY uk_mrat_token (token),
		INDEX idx_mrat_video_token (video_id, token),
		INDEX idx_mrat_review (review_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS manual_post_reviews (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		post_id BIGINT NOT NULL,
		requester_email VARCHAR(255) NOT NULL,
		title VARCHAR(255) NOT NULL DEFAULT '',
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		reviewer VARCHAR(100) NOT NULL DEFAULT '',
		review_note TEXT,
		reviewed_at DATETIME NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_mpr_status_created (status, created_at),
		INDEX idx_mpr_post (post_id),
		INDEX idx_mpr_requester (requester_email)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS manual_post_review_access_tokens (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		review_id BIGINT NOT NULL,
		post_id BIGINT NOT NULL,
		token VARCHAR(80) NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE KEY uk_mprat_token (token),
		INDEX idx_mprat_post_token (post_id, token),
		INDEX idx_mprat_review (review_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE posts ADD COLUMN category VARCHAR(100) NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") && !strings.Contains(err.Error(), "Unknown column") {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS system_notifications (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		INDEX idx_system_notifications_created_at (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS homepage_posters (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		image_url VARCHAR(500) NOT NULL,
		link_url VARCHAR(500) NOT NULL DEFAULT '',
		open_in_new_tab TINYINT(1) NOT NULL DEFAULT 0,
		enabled TINYINT(1) NOT NULL DEFAULT 1,
		sort_order INT NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_homepage_posters_enabled_sort (enabled, sort_order)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_reports (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		video_id VARCHAR(255) NOT NULL,
		reporter_email VARCHAR(255) NOT NULL,
		reason VARCHAR(500) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE KEY uk_vr_video_reporter (video_id, reporter_email),
		INDEX idx_vr_video (video_id),
		INDEX idx_vr_created (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_reports (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		post_id BIGINT NOT NULL,
		reporter_email VARCHAR(255) NOT NULL,
		reason VARCHAR(500) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE KEY uk_pr_post_reporter (post_id, reporter_email),
		INDEX idx_pr_post (post_id),
		INDEX idx_pr_created (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_bans (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		reason TEXT NOT NULL,
		banned_until DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		created_by_admin VARCHAR(255) NOT NULL DEFAULT '',
		pardoned_at DATETIME NULL,
		pardon_reason TEXT NULL,
		INDEX idx_user_bans_email (email),
		INDEX idx_user_bans_until (banned_until)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_mutes (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		reason TEXT NOT NULL,
		muted_until DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		created_by_admin VARCHAR(255) NOT NULL DEFAULT '',
		INDEX idx_user_mutes_email (email),
		INDEX idx_user_mutes_until (muted_until)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_system_messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		title VARCHAR(255) NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		INDEX idx_user_system_messages_email (email)
	)`)
	if err != nil {
		panic(err)
	}
	_, _ = db.Exec(`ALTER TABLE video_library ADD COLUMN takedown_reason TEXT NULL`)
	_, _ = db.Exec(`ALTER TABLE posts ADD COLUMN takedown_reason TEXT NULL`)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS report_reviews (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		target_type VARCHAR(20) NOT NULL,
		target_id VARCHAR(255) NOT NULL,
		title VARCHAR(255) NOT NULL DEFAULT '',
		report_count INT NOT NULL DEFAULT 0,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		reviewer VARCHAR(100) NOT NULL DEFAULT '',
		review_note TEXT,
		reviewed_at DATETIME NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_rr_status (status),
		INDEX idx_rr_target (target_type, target_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		email VARCHAR(255) PRIMARY KEY,
		nickname VARCHAR(100) NOT NULL DEFAULT '',
		password_hash VARCHAR(255) NOT NULL DEFAULT '',
		password_salt VARCHAR(255) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		balance DECIMAL(10,2) NOT NULL DEFAULT 0
	)`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN avatar_url VARCHAR(255) NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN banner_url VARCHAR(255) NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN notice TEXT`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN motto VARCHAR(40) NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN message_read_at DATETIME NULL`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN video_publish_credits INT NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN post_publish_credits INT NOT NULL DEFAULT 0`)
}

var sixDigitsRe = regexp.MustCompile(`^\d{6}$`)

func adminHashPassword(password string) (hash, salt string, err error) {
	if !sixDigitsRe.MatchString(password) {
		return "", "", errors.New("password must be 6 digits")
	}
	saltBytes := make([]byte, 16)
	if _, err = rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt = base64.RawURLEncoding.EncodeToString(saltBytes)
	sum := sha256.Sum256([]byte(salt + password))
	hash = base64.RawURLEncoding.EncodeToString(sum[:])
	return hash, salt, nil
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type HomepagePoster struct {
	ID           int64  `json:"id"`
	ImageURL     string `json:"imageUrl"`
	LinkURL      string `json:"linkUrl"`
	OpenInNewTab bool   `json:"openInNewTab"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sortOrder"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func listVideoCategories() ([]Category, error) {
	rows, err := db.Query("SELECT id, name FROM video_categories ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Category, 0)
	for rows.Next() {
		var item Category
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func buildMediaURL(categoryID, filename string) string {
	parts := []string{
		"/media",
		"videos",
		url.PathEscape(categoryID),
		url.PathEscape(filename),
	}
	return path.Join(parts...)
}

func handleAdminVideoCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := listVideoCategories()
		if err != nil {
			http.Error(w, "failed to load categories", http.StatusInternalServerError)
			return
		}
		writeJSON(w, list)
		return
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" || name == "all" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		var exists int
		if err := db.QueryRow("SELECT 1 FROM video_categories WHERE id = ?", name).Scan(&exists); err == nil {
			writeJSON(w, map[string]any{"status": "ok"})
			return
		} else if err != sql.ErrNoRows {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		target := filepath.Join("storage", "videos", name)
		if err := os.MkdirAll(target, 0o755); err != nil {
			http.Error(w, "failed to create category", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("INSERT INTO video_categories (id, name, created_at) VALUES (?, ?, ?)", name, name, time.Now()); err != nil {
			http.Error(w, "failed to create category", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
		return
	case http.MethodPut:
		var req struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := strings.TrimSpace(req.ID)
		name := strings.TrimSpace(req.Name)
		if id == "" || id == "all" || name == "" || name == "all" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		if id == name {
			writeJSON(w, map[string]any{"status": "ok"})
			return
		}
		var exists int
		if err := db.QueryRow("SELECT 1 FROM video_categories WHERE id = ?", name).Scan(&exists); err == nil {
			http.Error(w, "name exists", http.StatusBadRequest)
			return
		} else if err != sql.ErrNoRows {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		oldDir := filepath.Join("storage", "videos", id)
		newDir := filepath.Join("storage", "videos", name)
		if _, err := os.Stat(newDir); err == nil {
			http.Error(w, "target exists", http.StatusBadRequest)
			return
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			http.Error(w, "failed to rename category", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("UPDATE video_categories SET id = ?, name = ? WHERE id = ?", name, name, id); err != nil {
			http.Error(w, "failed to update category", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("UPDATE video_uploads SET category = ? WHERE category = ?", name, id); err != nil {
			http.Error(w, "failed to update videos", http.StatusInternalServerError)
			return
		}
		rows, err := db.Query("SELECT video_id, filename FROM video_library WHERE category = ?", id)
		if err != nil {
			http.Error(w, "failed to update videos", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var videoID, filename string
			if err := rows.Scan(&videoID, &filename); err != nil {
				continue
			}
			base := strings.TrimSuffix(filename, filepath.Ext(filename))
			thumbName := base + ".jpg"
			playURL := buildMediaURL(name, filename)
			thumbURL := buildMediaURL(name, thumbName)
			filePath := filepath.Join("storage", "videos", name, filename)
			if _, err := db.Exec("UPDATE video_library SET category = ?, play_url = ?, thumb_url = ?, file_path = ?, updated_at = ? WHERE video_id = ?",
				name, playURL, thumbURL, filePath, time.Now(), videoID); err != nil {
				http.Error(w, "failed to update videos", http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, map[string]any{"status": "ok"})
		return
	case http.MethodDelete:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := strings.TrimSpace(req.ID)
		if id == "" || id == "all" || strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var used int
		if err := db.QueryRow("SELECT 1 FROM video_library WHERE category = ? LIMIT 1", id).Scan(&used); err == nil {
			http.Error(w, "category in use", http.StatusBadRequest)
			return
		} else if err != sql.ErrNoRows {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		target := filepath.Join("storage", "videos", id)
		entries, err := os.ReadDir(target)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if len(entries) > 0 {
			http.Error(w, "category not empty", http.StatusBadRequest)
			return
		}
		if err := os.Remove(target); err != nil {
			http.Error(w, "failed to delete category", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("DELETE FROM video_categories WHERE id = ?", id); err != nil {
			http.Error(w, "failed to delete category", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminPostCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name FROM post_categories ORDER BY created_at DESC")
		if err != nil {
			http.Error(w, "failed to load categories", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		result := make([]Category, 0)
		for rows.Next() {
			var item Category
			if err := rows.Scan(&item.ID, &item.Name); err != nil {
				continue
			}
			result = append(result, item)
		}
		writeJSON(w, result)
		return
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" || name == "all" {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		var exists int
		err := db.QueryRow("SELECT 1 FROM post_categories WHERE id = ?", name).Scan(&exists)
		if err == nil {
			writeJSON(w, map[string]any{"status": "ok"})
			return
		}
		if err != sql.ErrNoRows {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("INSERT INTO post_categories (id, name, created_at) VALUES (?, ?, ?)", name, name, time.Now()); err != nil {
			http.Error(w, "failed to create category", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
		return
	case http.MethodPut:
		var req struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := strings.TrimSpace(req.ID)
		name := strings.TrimSpace(req.Name)
		if id == "" || id == "all" || name == "" || name == "all" {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if id == name {
			writeJSON(w, map[string]any{"status": "ok"})
			return
		}
		var exists int
		if err := db.QueryRow("SELECT 1 FROM post_categories WHERE id = ?", name).Scan(&exists); err == nil {
			http.Error(w, "name exists", http.StatusBadRequest)
			return
		} else if err != sql.ErrNoRows {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("UPDATE post_categories SET id = ?, name = ? WHERE id = ?", name, name, id); err != nil {
			http.Error(w, "failed to update category", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("UPDATE posts SET category = ? WHERE category = ?", name, id); err != nil {
			http.Error(w, "failed to update posts", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
		return
	case http.MethodDelete:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := strings.TrimSpace(req.ID)
		if id == "" || id == "all" {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var used int
		if err := db.QueryRow("SELECT 1 FROM posts WHERE category = ? LIMIT 1", id).Scan(&used); err == nil {
			http.Error(w, "category in use", http.StatusBadRequest)
			return
		} else if err != sql.ErrNoRows {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("DELETE FROM post_categories WHERE id = ?", id); err != nil {
			http.Error(w, "failed to delete category", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminSystemNotifications(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, title, content, created_at FROM system_notifications ORDER BY created_at DESC LIMIT 100")
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type sysNotif struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			Content   string `json:"content"`
			CreatedAt string `json:"createdAt"`
		}
		var list []sysNotif
		for rows.Next() {
			var n sysNotif
			var t time.Time
			if err := rows.Scan(&n.ID, &n.Title, &n.Content, &t); err != nil {
				continue
			}
			n.CreatedAt = t.Format("2006-01-02 15:04:05")
			list = append(list, n)
		}
		if list == nil {
			list = []sysNotif{}
		}
		writeJSON(w, list)
	case http.MethodPost:
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(req.Title)
		content := strings.TrimSpace(req.Content)
		if title == "" || content == "" {
			http.Error(w, "title and content required", http.StatusBadRequest)
			return
		}
		_, err := db.Exec("INSERT INTO system_notifications (title, content, created_at) VALUES (?, ?, ?)", title, content, time.Now())
		if err != nil {
			http.Error(w, "failed to create notification", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
	case http.MethodDelete:
		var req struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		_, err := db.Exec("DELETE FROM system_notifications WHERE id = ?", req.ID)
		if err != nil {
			http.Error(w, "failed to delete notification", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type manualVideoReviewItem struct {
	ID             int64  `json:"id"`
	VideoID        string `json:"videoId"`
	RequesterEmail string `json:"requesterEmail"`
	Title          string `json:"title"`
	CreatedAt      string `json:"createdAt"`
	WatchURL       string `json:"watchUrl"`
}

func ensureManualReviewAccessToken(reviewID int64, videoID string) (string, error) {
	var token string
	err := db.QueryRow(`SELECT token FROM manual_review_access_tokens
		WHERE review_id = ? AND video_id = ? AND expires_at > ?
		ORDER BY id DESC LIMIT 1`, reviewID, videoID, time.Now()).Scan(&token)
	if err == nil {
		return token, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = hex.EncodeToString(raw)
	now := time.Now()
	_, err = db.Exec(`INSERT INTO manual_review_access_tokens (review_id, video_id, token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`, reviewID, videoID, token, now.Add(24*time.Hour), now)
	if err != nil {
		return "", err
	}
	return token, nil
}

func forceDeleteVideo(videoID string) error {
	var category, filename string
	err := db.QueryRow("SELECT category, filename FROM video_uploads WHERE video_id = ?", videoID).Scan(&category, &filename)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		videoPath := filepath.Join("storage", "videos", category, filename)
		_ = os.Remove(videoPath)
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		thumbPath := filepath.Join("storage", "videos", category, base+".jpg")
		_ = os.Remove(thumbPath)
	}
	_, _ = db.Exec("DELETE FROM video_comment_likes WHERE comment_id IN (SELECT id FROM video_comments WHERE video_id = ?)", videoID)
	_, _ = db.Exec("DELETE FROM video_comments WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM video_views WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM video_favorites WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM video_likes WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM video_uploads WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM video_library WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM manual_review_access_tokens WHERE video_id = ?", videoID)
	return nil
}

func handleAdminManualVideoReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, video_id, requester_email, title, created_at
			FROM manual_video_reviews
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT 200`)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		list := make([]manualVideoReviewItem, 0)
		for rows.Next() {
			var item manualVideoReviewItem
			var t time.Time
			if err := rows.Scan(&item.ID, &item.VideoID, &item.RequesterEmail, &item.Title, &t); err != nil {
				continue
			}
			token, err := ensureManualReviewAccessToken(item.ID, item.VideoID)
			if err != nil {
				continue
			}
			item.WatchURL = "http://localhost:8080/player.html?id=" + url.QueryEscape(item.VideoID) + "&adminReviewToken=" + url.QueryEscape(token)
			item.CreatedAt = t.Format("2006-01-02 15:04:05")
			list = append(list, item)
		}
		writeJSON(w, list)
		return
	case http.MethodPost:
		var req struct {
			ID       int64  `json:"id"`
			Action   string `json:"action"`
			Reviewer string `json:"reviewer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action != "approve" && action != "reject" {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}
		reviewer := strings.TrimSpace(req.Reviewer)
		if reviewer == "" {
			reviewer = "admin"
		}

		var videoID, requesterEmail, title, status string
		err := db.QueryRow(`SELECT video_id, requester_email, title, status
			FROM manual_video_reviews
			WHERE id = ?`, req.ID).Scan(&videoID, &requesterEmail, &title, &status)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if status != "pending" {
			http.Error(w, "already handled", http.StatusBadRequest)
			return
		}

		now := time.Now()
		resultStatus := "approved"
		reason := "您的视频人工复审成功，视频继续保留。"
		if action == "reject" {
			resultStatus = "rejected"
			reason = "您的视频人工复审失败，视频已被强制清除。"
			if err := forceDeleteVideo(videoID); err != nil {
				http.Error(w, "failed to force delete video", http.StatusInternalServerError)
				return
			}
		}

		_, err = db.Exec(`UPDATE manual_video_reviews
			SET status = ?, reviewer = ?, reviewed_at = ?, updated_at = ?
			WHERE id = ?`, resultStatus, reviewer, now, now, req.ID)
		if err != nil {
			http.Error(w, "failed to update review status", http.StatusInternalServerError)
			return
		}

		if action == "approve" {
			_, _ = db.Exec("UPDATE video_library SET review_status = 'approved' WHERE video_id = ?", videoID)
		}
		_, _ = db.Exec(`INSERT INTO video_review_messages (video_id, email, title, result, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, videoID, requesterEmail, title, "manual_"+resultStatus, reason, now)
		writeJSON(w, map[string]any{
			"status": "ok",
		})
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type manualPostReviewItem struct {
	ID             int64  `json:"id"`
	PostID         int64  `json:"postId"`
	RequesterEmail string `json:"requesterEmail"`
	Title          string `json:"title"`
	CreatedAt      string `json:"createdAt"`
	WatchURL       string `json:"watchUrl"`
}

func ensureManualPostReviewAccessToken(reviewID, postID int64) (string, error) {
	var token string
	err := db.QueryRow(`SELECT token FROM manual_post_review_access_tokens
		WHERE review_id = ? AND post_id = ? AND expires_at > ?
		ORDER BY id DESC LIMIT 1`, reviewID, postID, time.Now()).Scan(&token)
	if err == nil {
		return token, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = hex.EncodeToString(raw)
	now := time.Now()
	_, err = db.Exec(`INSERT INTO manual_post_review_access_tokens (review_id, post_id, token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`, reviewID, postID, token, now.Add(24*time.Hour), now)
	if err != nil {
		return "", err
	}
	return token, nil
}

func handleAdminManualPostReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, post_id, requester_email, title, created_at
			FROM manual_post_reviews
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT 200`)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		list := make([]manualPostReviewItem, 0)
		for rows.Next() {
			var item manualPostReviewItem
			var t time.Time
			if err := rows.Scan(&item.ID, &item.PostID, &item.RequesterEmail, &item.Title, &t); err != nil {
				continue
			}
			token, err := ensureManualPostReviewAccessToken(item.ID, item.PostID)
			if err != nil {
				item.WatchURL = fmt.Sprintf("http://localhost:8080/post_detail.html?id=%d", item.PostID)
			} else {
				item.WatchURL = fmt.Sprintf("http://localhost:8080/post_detail.html?id=%d&adminReviewToken=%s", item.PostID, url.QueryEscape(token))
			}
			item.CreatedAt = t.Format("2006-01-02 15:04:05")
			list = append(list, item)
		}
		writeJSON(w, list)
		return
	case http.MethodPost:
		var req struct {
			ID       int64  `json:"id"`
			Action   string `json:"action"`
			Reviewer string `json:"reviewer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action != "approve" && action != "reject" {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}
		reviewer := strings.TrimSpace(req.Reviewer)
		if reviewer == "" {
			reviewer = "admin"
		}
		var postID int64
		var requesterEmail, title, status string
		err := db.QueryRow(`SELECT post_id, requester_email, title, status
			FROM manual_post_reviews
			WHERE id = ?`, req.ID).Scan(&postID, &requesterEmail, &title, &status)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if status != "pending" {
			http.Error(w, "already handled", http.StatusBadRequest)
			return
		}
		now := time.Now()
		resultStatus := "approved"
		reason := "您的帖子人工复审成功，已恢复展示。"
		if action == "reject" {
			resultStatus = "rejected"
			reason = "您的帖子人工复审未通过，维持当前状态。"
		}
		_, err = db.Exec(`UPDATE manual_post_reviews
			SET status = ?, reviewer = ?, reviewed_at = ?, updated_at = ?
			WHERE id = ?`, resultStatus, reviewer, now, now, req.ID)
		if err != nil {
			http.Error(w, "failed to update review status", http.StatusInternalServerError)
			return
		}
		if action == "approve" {
			_, _ = db.Exec("UPDATE posts SET review_status = 'approved' WHERE id = ?", postID)
		}
		_, _ = db.Exec(`INSERT INTO post_review_messages (post_id, email, title, result, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, postID, requesterEmail, title, "manual_"+resultStatus, reason, now)
		writeJSON(w, map[string]any{
			"status": "ok",
		})
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type reportReviewItem struct {
	ID          int64  `json:"id"`
	TargetType  string `json:"targetType"`
	TargetID    string `json:"targetId"`
	Title       string `json:"title"`
	ReportCount int    `json:"reportCount"`
	CreatedAt   string `json:"createdAt"`
	WatchURL    string `json:"watchUrl"`
}

func handleAdminReportReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, target_type, target_id, title, report_count, created_at
			FROM report_reviews
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT 200`)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		list := make([]reportReviewItem, 0)
		for rows.Next() {
			var item reportReviewItem
			var t time.Time
			if err := rows.Scan(&item.ID, &item.TargetType, &item.TargetID, &item.Title, &item.ReportCount, &t); err != nil {
				continue
			}
			item.CreatedAt = t.Format("2006-01-02 15:04:05")
			if item.TargetType == "video" {
				item.WatchURL = "http://localhost:8080/player.html?id=" + url.QueryEscape(item.TargetID)
			} else {
				item.WatchURL = "http://localhost:8080/post_detail.html?id=" + url.QueryEscape(item.TargetID)
			}
			list = append(list, item)
		}
		writeJSON(w, list)
		return
	case http.MethodPost:
		var req struct {
			ID       int64  `json:"id"`
			Action   string `json:"action"`
			Reviewer string `json:"reviewer"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action != "keep" && action != "takedown" {
			http.Error(w, "invalid action, must be 'keep' or 'takedown'", http.StatusBadRequest)
			return
		}
		reviewer := strings.TrimSpace(req.Reviewer)
		if reviewer == "" {
			reviewer = "admin"
		}
		reason := strings.TrimSpace(req.Reason)
		if action == "takedown" && reason == "" {
			http.Error(w, "takedown requires a reason", http.StatusBadRequest)
			return
		}

		var targetType, targetID, title, status string
		err := db.QueryRow(`SELECT target_type, target_id, title, status
			FROM report_reviews WHERE id = ?`, req.ID).Scan(&targetType, &targetID, &title, &status)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if status != "pending" {
			http.Error(w, "already handled", http.StatusBadRequest)
			return
		}

		now := time.Now()
		resultStatus := "kept"
		var msgReason string
		if action == "keep" {
			msgReason = "经复审，您的内容未违反社区规范，继续保留。"
		} else {
			resultStatus = "taken_down"
			msgReason = fmt.Sprintf("您的内容因举报复审已被下架，原因：%s", reason)
			if targetType == "video" {
				if err := forceDeleteVideo(targetID); err != nil {
					http.Error(w, "failed to takedown video", http.StatusInternalServerError)
					return
				}
			} else {
				_, _ = db.Exec("DELETE FROM posts WHERE id = ?", targetID)
				_, _ = db.Exec("DELETE FROM post_comments WHERE post_id = ?", targetID)
				_, _ = db.Exec("DELETE FROM post_likes WHERE post_id = ?", targetID)
				_, _ = db.Exec("DELETE FROM post_views WHERE post_id = ?", targetID)
				_, _ = db.Exec("DELETE FROM post_favorites WHERE post_id = ?", targetID)
				_, _ = db.Exec("DELETE FROM post_reports WHERE post_id = ?", targetID)
			}
		}

		_, err = db.Exec(`UPDATE report_reviews
			SET status = ?, reviewer = ?, review_note = ?, reviewed_at = ?, updated_at = ?
			WHERE id = ?`, resultStatus, reviewer, reason, now, now, req.ID)
		if err != nil {
			http.Error(w, "failed to update", http.StatusInternalServerError)
			return
		}

		var authorEmail string
		if targetType == "video" {
			_ = db.QueryRow("SELECT COALESCE(author_email, '') FROM video_library WHERE video_id = ?", targetID).Scan(&authorEmail)
		} else {
			_ = db.QueryRow("SELECT COALESCE(email, '') FROM posts WHERE id = ?", targetID).Scan(&authorEmail)
		}
		if authorEmail != "" {
			_, _ = db.Exec(`INSERT INTO video_review_messages (video_id, email, title, result, reason, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`, targetID, authorEmail, title, "report_"+resultStatus, msgReason, now)
		}

		writeJSON(w, map[string]any{"status": "ok"})
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listHomepagePosters(includeDisabled bool) ([]HomepagePoster, error) {
	query := `SELECT id, image_url, link_url, open_in_new_tab, enabled, sort_order, created_at, updated_at
		FROM homepage_posters`
	if !includeDisabled {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY sort_order DESC, id DESC"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]HomepagePoster, 0)
	for rows.Next() {
		var item HomepagePoster
		var openInt, enabledInt int
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.ImageURL, &item.LinkURL, &openInt, &enabledInt, &item.SortOrder, &createdAt, &updatedAt); err != nil {
			continue
		}
		item.OpenInNewTab = openInt == 1
		item.Enabled = enabledInt == 1
		item.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		item.UpdatedAt = updatedAt.Format("2006-01-02 15:04:05")
		result = append(result, item)
	}
	return result, nil
}

func handleAdminHomepagePosters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := listHomepagePosters(true)
		if err != nil {
			http.Error(w, "failed to load posters", http.StatusInternalServerError)
			return
		}
		writeJSON(w, list)
	case http.MethodPut:
		var req struct {
			ID           int64  `json:"id"`
			LinkURL      string `json:"linkUrl"`
			OpenInNewTab bool   `json:"openInNewTab"`
			Enabled      bool   `json:"enabled"`
			SortOrder    int    `json:"sortOrder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		linkURL := strings.TrimSpace(req.LinkURL)
		if linkURL != "" && !strings.HasPrefix(linkURL, "http://") && !strings.HasPrefix(linkURL, "https://") {
			http.Error(w, "link must start with http:// or https://", http.StatusBadRequest)
			return
		}
		enabledInt := 0
		if req.Enabled {
			enabledInt = 1
		}
		openInt := 0
		if req.OpenInNewTab {
			openInt = 1
		}
		_, err := db.Exec(`UPDATE homepage_posters
			SET link_url = ?, open_in_new_tab = ?, enabled = ?, sort_order = ?, updated_at = ?
			WHERE id = ?`, linkURL, openInt, enabledInt, req.SortOrder, time.Now(), req.ID)
		if err != nil {
			http.Error(w, "failed to update poster", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok"})
	case http.MethodDelete:
		var req struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var imageURL string
		if err := db.QueryRow("SELECT image_url FROM homepage_posters WHERE id = ?", req.ID).Scan(&imageURL); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec("DELETE FROM homepage_posters WHERE id = ?", req.ID); err != nil {
			http.Error(w, "failed to delete poster", http.StatusInternalServerError)
			return
		}
		if strings.HasPrefix(imageURL, "/media/posters/") {
			filename := strings.TrimPrefix(imageURL, "/media/posters/")
			if filename != "" && !strings.Contains(filename, "/") && !strings.Contains(filename, "\\") {
				_ = os.Remove(filepath.Join("storage", "posters", filename))
			}
		}
		writeJSON(w, map[string]any{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminHomepagePosterUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
		http.Error(w, "only image files allowed", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Join("storage", "posters"), 0o755); err != nil {
		http.Error(w, "failed to create poster dir", http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dstPath := filepath.Join("storage", "posters", filename)
	out, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	linkURL := strings.TrimSpace(r.FormValue("linkUrl"))
	if linkURL != "" && !strings.HasPrefix(linkURL, "http://") && !strings.HasPrefix(linkURL, "https://") {
		http.Error(w, "link must start with http:// or https://", http.StatusBadRequest)
		return
	}
	openInNewTab := strings.EqualFold(r.FormValue("openInNewTab"), "true") || r.FormValue("openInNewTab") == "1"
	enabled := !strings.EqualFold(r.FormValue("enabled"), "false") && r.FormValue("enabled") != "0"
	sortOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("sortOrder")))
	openInt := 0
	if openInNewTab {
		openInt = 1
	}
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := time.Now()
	imageURL := "/media/posters/" + filename
	res, err := db.Exec(`INSERT INTO homepage_posters
		(image_url, link_url, open_in_new_tab, enabled, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		imageURL, linkURL, openInt, enabledInt, sortOrder, now, now)
	if err != nil {
		http.Error(w, "failed to save poster", http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, map[string]any{
		"status":   "ok",
		"id":       id,
		"imageUrl": imageURL,
	})
}

// ---------- 批量上传、搜索下架、用户封禁/禁言/赦免 ----------

func adminBuildVideoID(categoryID, filename string) string {
	ext := path.Ext(filename)
	base := filename
	if ext != "" {
		base = strings.TrimSuffix(filename, ext)
	}
	return fmt.Sprintf("%s_%s", categoryID, base)
}

type batchFile struct {
	tempPath string
	origName string
	size     int64
}

func handleAdminBatchUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		writeJSONErr(w, "parse form failed", http.StatusBadRequest)
		return
	}
	var category, authorEmail, authorNickname string
	var collected []batchFile
	allowedExt := map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSONErr(w, "parse form failed", http.StatusBadRequest)
			return
		}
		formName := part.FormName()
		filename := part.FileName()
		origName := filepath.Base(filename)
		ext := strings.ToLower(path.Ext(origName))
		hasValidExt := origName != "" && !strings.Contains(origName, "..") && allowedExt[ext]
		isFilePart := hasValidExt || (formName == "files" || formName == "file")
		if isFilePart {
			if origName == "" || strings.Contains(origName, "..") {
				origName = "video.mp4"
				ext = ".mp4"
			} else if ext == "" {
				origName = origName + ".mp4"
				ext = ".mp4"
			}
			if !allowedExt[ext] {
				_, _ = io.Copy(io.Discard, part)
				part.Close()
				continue
			}
			tmp, err := os.CreateTemp("", "batch-*"+ext)
			if err != nil {
				_, _ = io.Copy(io.Discard, part)
				part.Close()
				continue
			}
			size, _ := io.Copy(tmp, part)
			tmp.Close()
			part.Close()
			if size == 0 {
				os.Remove(tmp.Name())
				continue
			}
			collected = append(collected, batchFile{tempPath: tmp.Name(), origName: origName, size: size})
			continue
		}
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, part)
		part.Close()
		val := strings.TrimSpace(buf.String())
		switch formName {
		case "category":
			category = val
		case "authorEmail":
			authorEmail = val
		case "authorNickname":
			authorNickname = val
		}
	}
	if category == "" || category == "all" || strings.ContainsAny(category, "../\\") {
		for _, f := range collected {
			os.Remove(f.tempPath)
		}
		writeJSONErr(w, "请选择视频分类", http.StatusBadRequest)
		return
	}
	if authorEmail == "" {
		for _, f := range collected {
			os.Remove(f.tempPath)
		}
		writeJSONErr(w, "请填写作者邮箱，视频将归属该用户，否则不会出现在任何用户主页", http.StatusBadRequest)
		return
	}
	if len(collected) == 0 {
		writeJSONErr(w, "未收到视频文件，请选择 .mp4/.mov/.mkv/.webm 文件", http.StatusBadRequest)
		return
	}
	targetDir := filepath.Join("storage", "videos", category)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		for _, f := range collected {
			os.Remove(f.tempPath)
		}
		writeJSONErr(w, "创建分类目录失败", http.StatusInternalServerError)
		return
	}
	var uploaded []string
	for _, f := range collected {
		ext := strings.ToLower(path.Ext(f.origName))
		base := strings.TrimSuffix(f.origName, ext)
		id := generateRandomID(4)
		finalName := fmt.Sprintf("%s_%s%s", id, base, ext)
		dstPath := filepath.Join(targetDir, finalName)
		if err := os.Rename(f.tempPath, dstPath); err != nil {
			// 跨盘时 Rename 可能失败，改为复制后删除
			src, oerr := os.Open(f.tempPath)
			if oerr != nil {
				os.Remove(f.tempPath)
				continue
			}
			out, cerr := os.Create(dstPath)
			if cerr != nil {
				src.Close()
				os.Remove(f.tempPath)
				continue
			}
			_, _ = io.Copy(out, src)
			out.Close()
			src.Close()
			os.Remove(f.tempPath)
		}
		now := time.Now()
		videoID := adminBuildVideoID(category, finalName)
		playURL := buildMediaURL(category, finalName)
		thumbURL := playURL
		filePath := dstPath
		_, _ = db.Exec("INSERT INTO video_uploads (video_id, email, nickname, category, filename, size_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			videoID, authorEmail, authorNickname, category, finalName, f.size, now)
		_, _ = db.Exec(`INSERT INTO video_library (video_id, category, title, description, tags, filename, file_path, thumb_url, play_url, duration_sec, size_bytes, format_json, created_at, updated_at, author_email, author_nickname, review_status)
			VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, 0, ?, '', ?, ?, ?, ?, 'approved')
			ON DUPLICATE KEY UPDATE category=VALUES(category), title=VALUES(title), filename=VALUES(filename), file_path=VALUES(file_path), thumb_url=VALUES(thumb_url), play_url=VALUES(play_url), size_bytes=VALUES(size_bytes), updated_at=VALUES(updated_at), author_email=VALUES(author_email), author_nickname=VALUES(author_nickname), review_status='approved'`,
			videoID, category, base, finalName, filePath, thumbURL, playURL, f.size, now, now, authorEmail, authorNickname)
		uploaded = append(uploaded, videoID)
	}
	writeJSON(w, map[string]any{"status": "ok", "uploaded": uploaded, "count": len(uploaded)})
}

func handleAdminSearchVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	query := `SELECT v.video_id, v.title, v.category, COALESCE(v.review_status,'approved') AS review_status, COALESCE(v.takedown_reason,'') AS takedown_reason
		FROM video_library v WHERE 1=1`
	args := []any{}
	if q != "" {
		query += ` AND (v.video_id LIKE ? OR v.title LIKE ? OR v.description LIKE ? OR v.category LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	query += " ORDER BY v.created_at DESC LIMIT 100"
	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var videoID, title, category, reviewStatus, takedownReason string
		if err := rows.Scan(&videoID, &title, &category, &reviewStatus, &takedownReason); err != nil {
			continue
		}
		list = append(list, map[string]any{
			"videoId":        videoID,
			"title":         title,
			"category":      category,
			"reviewStatus":  reviewStatus,
			"takedownReason": takedownReason,
		})
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, list)
}

func handleAdminSearchPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	query := `SELECT p.id, p.title, p.email, p.nickname, COALESCE(p.review_status,'approved') AS review_status, COALESCE(p.takedown_reason,'') AS takedown_reason
		FROM posts p WHERE 1=1`
	args := []any{}
	if q != "" {
		query += ` AND (p.title LIKE ? OR p.content_text LIKE ? OR p.content LIKE ? OR p.nickname LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	query += " ORDER BY p.created_at DESC LIMIT 100"
	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id int64
		var title, email, nickname, reviewStatus, takedownReason string
		if err := rows.Scan(&id, &title, &email, &nickname, &reviewStatus, &takedownReason); err != nil {
			continue
		}
		list = append(list, map[string]any{
			"postId":         id,
			"title":         title,
			"email":         email,
			"nickname":      nickname,
			"reviewStatus":  reviewStatus,
			"takedownReason": takedownReason,
		})
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, list)
}

func handleAdminTakedownVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		VideoID string `json:"videoId"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	videoID := strings.TrimSpace(req.VideoID)
	reason := strings.TrimSpace(req.Reason)
	if videoID == "" || reason == "" {
		http.Error(w, "videoId and reason required", http.StatusBadRequest)
		return
	}
	_, err := db.Exec("UPDATE video_library SET review_status = 'takedown', takedown_reason = ? WHERE video_id = ?", reason, videoID)
	if err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	var authorEmail string
	var title string
	_ = db.QueryRow("SELECT COALESCE(author_email, ''), COALESCE(title, '') FROM video_library WHERE video_id = ?", videoID).Scan(&authorEmail, &title)
	if authorEmail == "" {
		_ = db.QueryRow("SELECT email FROM video_uploads WHERE video_id = ?", videoID).Scan(&authorEmail)
	}
	if authorEmail != "" {
		msgTitle := "视频已下架"
		msgContent := fmt.Sprintf("您的视频《%s》已被下架。原因：%s", title, reason)
		_, _ = db.Exec("INSERT INTO user_system_messages (email, title, content, created_at) VALUES (?, ?, ?, ?)", authorEmail, msgTitle, msgContent, time.Now())
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleAdminTakedownPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PostID int64  `json:"postId"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if req.PostID <= 0 || reason == "" {
		http.Error(w, "postId and reason required", http.StatusBadRequest)
		return
	}
	_, err := db.Exec("UPDATE posts SET review_status = 'takedown', takedown_reason = ? WHERE id = ?", reason, req.PostID)
	if err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	var authorEmail, title string
	_ = db.QueryRow("SELECT email, COALESCE(title, '') FROM posts WHERE id = ?", req.PostID).Scan(&authorEmail, &title)
	if authorEmail != "" {
		msgTitle := "帖子已下架"
		msgContent := fmt.Sprintf("您的帖子《%s》已被下架。原因：%s", title, reason)
		_, _ = db.Exec("INSERT INTO user_system_messages (email, title, content, created_at) VALUES (?, ?, ?, ?)", authorEmail, msgTitle, msgContent, time.Now())
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleAdminSearchUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	query := `SELECT email, nickname FROM users WHERE 1=1`
	args := []any{}
	if q != "" {
		query += ` AND (email LIKE ? OR nickname LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	query += " LIMIT 50"
	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var email, nickname string
		if err := rows.Scan(&email, &nickname); err != nil {
			continue
		}
		list = append(list, map[string]any{"email": email, "nickname": nickname})
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, list)
}

func handleAdminUsersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := db.Query("SELECT email, nickname FROM users ORDER BY created_at DESC LIMIT 500")
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []map[string]string
	for rows.Next() {
		var email, nickname string
		if err := rows.Scan(&email, &nickname); err != nil {
			continue
		}
		list = append(list, map[string]string{"email": email, "nickname": nickname})
	}
	if list == nil {
		list = []map[string]string{}
	}
	writeJSON(w, list)
}

func handleAdminUsersCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Nickname string `json:"nickname"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	nickname := strings.TrimSpace(req.Nickname)
	password := strings.TrimSpace(req.Password)
	if email == "" {
		writeJSONErr(w, "email required", http.StatusBadRequest)
		return
	}
	hash, salt, err := adminHashPassword(password)
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	if nickname == "" {
		nickname = email
	}
	var exists int
	if err := db.QueryRow("SELECT 1 FROM users WHERE email = ?", email).Scan(&exists); err == nil {
		writeJSONErr(w, "email already registered", http.StatusConflict)
		return
	}
	now := time.Now()
	_, err = db.Exec("INSERT INTO users (email, nickname, password_hash, password_salt, created_at, balance) VALUES (?, ?, ?, ?, ?, ?)",
		email, nickname, hash, salt, now, 0)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "email": email, "nickname": nickname})
}

func writeJSONErr(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func handleAdminUserBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email       string `json:"email"`
		Reason      string `json:"reason"`
		BannedUntil string `json:"bannedUntil"` // 如 "2025-03-08 00:00:00"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	reason := strings.TrimSpace(req.Reason)
	bannedUntil := strings.TrimSpace(req.BannedUntil)
	if email == "" || reason == "" || bannedUntil == "" {
		http.Error(w, "email, reason and bannedUntil required", http.StatusBadRequest)
		return
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", bannedUntil, time.Local)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02", bannedUntil, time.Local)
		if err != nil {
			http.Error(w, "bannedUntil format: 2006-01-02 15:04:05 or 2006-01-02", http.StatusBadRequest)
			return
		}
	}
	now := time.Now()
	_, err = db.Exec("INSERT INTO user_bans (email, reason, banned_until, created_at, created_by_admin) VALUES (?, ?, ?, ?, ?)",
		email, reason, t, now, "admin")
	if err != nil {
		http.Error(w, "insert failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleAdminUserMute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email     string `json:"email"`
		Reason    string `json:"reason"`
		MutedUntil string `json:"mutedUntil"` // 如 "2025-03-08 00:00:00"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	reason := strings.TrimSpace(req.Reason)
	mutedUntil := strings.TrimSpace(req.MutedUntil)
	if email == "" || reason == "" || mutedUntil == "" {
		http.Error(w, "email, reason and mutedUntil required", http.StatusBadRequest)
		return
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", mutedUntil, time.Local)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02", mutedUntil, time.Local)
		if err != nil {
			http.Error(w, "mutedUntil format: 2006-01-02 15:04:05 or 2006-01-02", http.StatusBadRequest)
			return
		}
	}
	now := time.Now()
	_, err = db.Exec("INSERT INTO user_mutes (email, reason, muted_until, created_at, created_by_admin) VALUES (?, ?, ?, ?, ?)",
		email, reason, t, now, "admin")
	if err != nil {
		http.Error(w, "insert failed", http.StatusInternalServerError)
		return
	}
	title := "禁言通知"
	content := fmt.Sprintf("您已被禁言至 %s。原因：%s。禁言期间无法评论与发布视频/帖子。", t.Format("2006-01-02 15:04:05"), reason)
	_, _ = db.Exec("INSERT INTO user_system_messages (email, title, content, created_at) VALUES (?, ?, ?, ?)", email, title, content, now)
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleAdminUserPardon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email string `json:"email"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	reason := strings.TrimSpace(req.Reason)
	if email == "" || reason == "" {
		http.Error(w, "email and reason required", http.StatusBadRequest)
		return
	}
	now := time.Now()
	res, err := db.Exec("UPDATE user_bans SET pardoned_at = ?, pardon_reason = ? WHERE email = ? AND pardoned_at IS NULL AND banned_until > ?", now, reason, email, now)
	if err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "no active ban to pardon", http.StatusBadRequest)
		return
	}
	title := "封号赦免通知"
	content := fmt.Sprintf("您的封号已被提前解除。原因：%s。", reason)
	_, _ = db.Exec("INSERT INTO user_system_messages (email, title, content, created_at) VALUES (?, ?, ?, ?)", email, title, content, now)
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, adminPage)
}

const adminPage = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>管理后台</title>
<style>
body { font-family: Arial, sans-serif; background: #f5f6f7; margin: 0; padding: 30px; }
.container { max-width: 980px; margin: 0 auto; background: #fff; padding: 20px; border-radius: 8px; box-shadow: 0 2px 6px rgba(0,0,0,0.1); }
.section { margin-bottom: 30px; }
.section h2 { margin: 0 0 12px 0; font-size: 18px; }
.row { display: flex; gap: 10px; margin-bottom: 12px; align-items: center; }
.row input, .row textarea { flex: 1; padding: 8px 10px; border: 1px solid #ddd; border-radius: 4px; font-family: inherit; }
.row textarea { min-height: 60px; resize: vertical; }
.row button { padding: 8px 14px; border: none; border-radius: 4px; background: #00AEEC; color: #fff; cursor: pointer; }
.list { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 10px; }
.item { background: #f6f7f8; border: 1px solid #e3e5e7; border-radius: 6px; padding: 8px 10px; display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.item span { font-size: 13px; color: #333; }
.item button { background: #f25d8e; border: none; color: #fff; padding: 4px 10px; border-radius: 4px; cursor: pointer; }
.error { color: #f25d8e; font-size: 12px; margin-top: 6px; min-height: 16px; }
.notif-list { display: flex; flex-direction: column; gap: 10px; }
.notif-item { background: #f6f7f8; border: 1px solid #e3e5e7; border-radius: 6px; padding: 12px 14px; display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.notif-item .notif-info { flex: 1; min-width: 0; }
.notif-item .notif-title { font-size: 14px; font-weight: 600; color: #333; margin-bottom: 4px; }
.notif-item .notif-content { font-size: 13px; color: #666; line-height: 1.5; white-space: pre-wrap; word-break: break-all; }
.notif-item .notif-time { font-size: 12px; color: #999; margin-top: 4px; }
.poster-list { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
.poster-item { border: 1px solid #e5e7eb; border-radius: 8px; background: #fff; overflow: hidden; }
.poster-thumb { width: 100%; aspect-ratio: 16/10; object-fit: cover; display: block; background: #f3f4f6; }
.poster-body { padding: 10px; font-size: 12px; color: #374151; display: grid; gap: 8px; }
.poster-actions { display: flex; gap: 8px; padding: 0 10px 10px; }
.checkbox-inline { display: flex; align-items: center; gap: 6px; font-size: 13px; color: #374151; white-space: nowrap; }
.review-list { display: flex; flex-direction: column; gap: 10px; }
.review-item { border: 1px solid #e5e7eb; border-radius: 8px; background: #fff; padding: 12px; display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; }
.review-main { flex: 1; min-width: 0; }
.review-title { font-weight: 600; color: #111827; margin-bottom: 6px; }
.review-meta { font-size: 12px; color: #6b7280; line-height: 1.6; }
.review-actions { display: flex; gap: 8px; }
.review-watch { background: #fff; border: 1px solid #d1d5db; color: #111827; text-decoration: none; padding: 0 10px; height: 32px; line-height: 30px; border-radius: 6px; font-size: 12px; }
.review-pass { background: #16a34a; border: none; color: #fff; width: 36px; height: 32px; border-radius: 6px; cursor: pointer; font-size: 20px; line-height: 1; }
.review-reject { background: #dc2626; border: none; color: #fff; width: 36px; height: 32px; border-radius: 6px; cursor: pointer; font-size: 20px; line-height: 1; }
</style>
</head>
<body>
<div class="container">
  <div class="section" style="display:flex;align-items:center;gap:12px;">
    <label for="adminLangSelect">语言 / Language:</label>
    <select id="adminLangSelect">
      <option value="zh-CN">简体中文</option>
      <option value="en-US">English</option>
      <option value="ru-RU">Русский</option>
    </select>
  </div>
  <div class="section">
    <h2 data-i18n="videoCategory">视频分类</h2>
    <div class="row">
      <input id="videoInput" data-i18n-placeholder="inputVideoCategory" placeholder="输入视频分类名称" />
      <button id="videoAddBtn" data-i18n="add">添加</button>
    </div>
    <div id="videoError" class="error"></div>
    <div id="videoList" class="list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="postCategory">帖子分类</h2>
    <div class="row">
      <input id="postInput" data-i18n-placeholder="inputPostCategory" placeholder="输入帖子分类名称" />
      <button id="postAddBtn" data-i18n="add">添加</button>
    </div>
    <div id="postError" class="error"></div>
    <div id="postList" class="list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="systemNotif">系统通知</h2>
    <div class="row">
      <input id="notifTitleInput" data-i18n-placeholder="notifTitle" placeholder="通知标题" />
    </div>
    <div class="row">
      <textarea id="notifContentInput" data-i18n-placeholder="notifContent" placeholder="通知内容"></textarea>
      <button id="notifAddBtn" style="align-self:flex-end;" data-i18n="send">发送</button>
    </div>
    <div id="notifError" class="error"></div>
    <div id="notifList" class="notif-list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="manualReviewVideo">视频人工复审区</h2>
    <div id="manualReviewError" class="error"></div>
    <div id="manualReviewList" class="review-list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="manualReviewPost">帖子人工复审区</h2>
    <div id="manualPostReviewError" class="error"></div>
    <div id="manualPostReviewList" class="review-list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="reportReview">举报复审区</h2>
    <div id="reportReviewError" class="error"></div>
    <div id="reportReviewList" class="review-list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="homepagePoster">首页海报</h2>
    <div class="row">
      <input id="posterFileInput" type="file" accept="image/*" />
      <input id="posterLinkInput" placeholder="https://example.com" />
    </div>
    <div class="row">
      <input id="posterSortInput" type="number" data-i18n-placeholder="posterSort" placeholder="排序值（越大越靠前）" value="0" />
      <label class="checkbox-inline"><input id="posterOpenBlankInput" type="checkbox" checked /><span data-i18n="posterOpenNew">新窗口打开</span></label>
      <label class="checkbox-inline"><input id="posterEnabledInput" type="checkbox" checked /><span data-i18n="posterEnabled">启用</span></label>
      <button id="posterUploadBtn" data-i18n="uploadPoster">上传海报</button>
    </div>
    <div id="posterError" class="error"></div>
    <div id="posterList" class="poster-list"></div>
  </div>
  <div class="section">
    <h2 data-i18n="batchUpload">批量上传视频</h2>
    <div class="row">
      <select id="batchCategorySelect"><option value="" data-i18n-opt="selectCategory">选择分类</option></select>
      <select id="batchAuthorSelect" style="min-width:180px;"><option value="">-- 选择作者 --</option></select>
      <input id="batchAuthorEmail" data-i18n-placeholder="authorEmailPlaceholder" placeholder="或填写作者邮箱（必填）" style="min-width:200px;" />
      <input id="batchAuthorNickname" data-i18n-placeholder="authorNickname" placeholder="作者昵称（可选）" />
    </div>
    <div class="row">
      <input id="batchFileInput" type="file" multiple accept=".mp4,.mov,.mkv,.webm" />
      <button id="batchUploadBtn" data-i18n="batchUploadBtn">批量上传</button>
    </div>
    <div id="batchError" class="error"></div>
    <div id="batchResult" style="font-size:12px;color:#666;"></div>
  </div>
  <div class="section">
    <h2 data-i18n="createUser">创建用户（无需邮箱验证）</h2>
    <p style="font-size:13px;color:#666;" data-i18n="createUserDesc">用于发布视频时选择作者，视频将只显示在该用户个人主页。</p>
    <div class="row">
      <input id="createUserEmail" placeholder="邮箱" style="width:180px;" />
      <input id="createUserNickname" placeholder="昵称" style="width:120px;" />
      <input id="createUserPassword" type="password" placeholder="密码（6位数字）" style="width:120px;" />
      <button id="createUserBtn">创建</button>
    </div>
    <div id="createUserError" class="error"></div>
  </div>
  <div class="section">
    <h2 data-i18n="takedown">搜索并下架视频/帖子</h2>
    <p style="font-size:13px;color:#666;" data-i18n="takedownDesc">下架需填写具体原因，用户访问时将看到原因。</p>
    <div class="row">
      <input id="takedownSearchInput" data-i18n-placeholder="searchPlaceholder" placeholder="搜索视频或帖子关键词" />
      <button id="takedownSearchVideosBtn" data-i18n="searchVideos">搜视频</button>
      <button id="takedownSearchPostsBtn" data-i18n="searchPosts">搜帖子</button>
    </div>
    <div id="takedownError" class="error"></div>
    <div id="takedownVideoList" class="review-list" style="margin-top:10px;"></div>
    <div id="takedownPostList" class="review-list" style="margin-top:10px;"></div>
  </div>
  <div class="section">
    <h2 data-i18n="userManage">用户管理：封号 / 禁言 / 赦免</h2>
    <p style="font-size:13px;color:#666;" data-i18n="userManageDesc">封号：登录后秒退并弹窗显示原因与剩余时间。禁言：消息通知禁言至某日及原因，期间不可评论与发视频/帖子。赦免：提前解除封号，需填写原因。</p>
    <div class="row">
      <input id="userSearchInput" data-i18n-placeholder="searchUser" placeholder="搜索用户（邮箱或昵称）" />
      <button id="userSearchBtn" data-i18n="searchUserBtn">搜索用户</button>
    </div>
    <div id="userManageError" class="error"></div>
    <div id="userList" class="review-list" style="margin-top:10px;"></div>
  </div>
</div>
<script>
const videoInput = document.getElementById('videoInput');
const videoAddBtn = document.getElementById('videoAddBtn');
const videoList = document.getElementById('videoList');
const videoError = document.getElementById('videoError');
const postInput = document.getElementById('postInput');
const postAddBtn = document.getElementById('postAddBtn');
const postList = document.getElementById('postList');
const postError = document.getElementById('postError');
const notifTitleInput = document.getElementById('notifTitleInput');
const notifContentInput = document.getElementById('notifContentInput');
const notifAddBtn = document.getElementById('notifAddBtn');
const notifListEl = document.getElementById('notifList');
const notifError = document.getElementById('notifError');
const manualReviewListEl = document.getElementById('manualReviewList');
const manualReviewError = document.getElementById('manualReviewError');
const manualPostReviewListEl = document.getElementById('manualPostReviewList');
const manualPostReviewError = document.getElementById('manualPostReviewError');
const reportReviewListEl = document.getElementById('reportReviewList');
const reportReviewError = document.getElementById('reportReviewError');
const posterFileInput = document.getElementById('posterFileInput');
const posterLinkInput = document.getElementById('posterLinkInput');
const posterSortInput = document.getElementById('posterSortInput');
const posterOpenBlankInput = document.getElementById('posterOpenBlankInput');
const posterEnabledInput = document.getElementById('posterEnabledInput');
const posterUploadBtn = document.getElementById('posterUploadBtn');
const posterListEl = document.getElementById('posterList');
const posterError = document.getElementById('posterError');

function setError(el, msg){ if(el) el.textContent = msg || ''; }
function escapeHtml(text){ const d=document.createElement('div'); d.textContent=text||''; return d.innerHTML; }

function renderList(container, list, onDelete, onEdit){
  container.innerHTML='';
  (list||[]).forEach(function(item){
    const row=document.createElement('div'); row.className='item';
    const text=document.createElement('span'); text.textContent=item.name||item.id;
    const actions=document.createElement('div'); actions.style.display='flex'; actions.style.gap='6px';
    const edit=document.createElement('button'); edit.textContent='编辑'; edit.style.background='#00AEEC'; edit.onclick=function(){ onEdit(item.id, item.name||item.id); };
    const del=document.createElement('button'); del.textContent='删除'; del.onclick=function(){ onDelete(item.id); };
    actions.appendChild(edit); actions.appendChild(del); row.appendChild(text); row.appendChild(actions); container.appendChild(row);
  });
}

async function loadVideoCategories(){
  try{ const res=await fetch('/api/video-categories'); const list=await res.json(); renderList(videoList, list, deleteVideoCategory, editVideoCategory); }
  catch(e){ setError(videoError,'加载失败'); }
}
async function loadPostCategories(){
  try{ const res=await fetch('/api/post-categories'); const list=await res.json(); renderList(postList, list, deletePostCategory, editPostCategory); }
  catch(e){ setError(postError,'加载失败'); }
}
async function addVideoCategory(){
  const name=(videoInput.value||'').trim(); if(!name){ setError(videoError,'请输入分类名称'); return; }
  const res=await fetch('/api/video-categories',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name})});
  if(!res.ok){ setError(videoError,await res.text()); return; }
  videoInput.value=''; setError(videoError,''); loadVideoCategories();
}
async function addPostCategory(){
  const name=(postInput.value||'').trim(); if(!name){ setError(postError,'请输入分类名称'); return; }
  const res=await fetch('/api/post-categories',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name})});
  if(!res.ok){ setError(postError,await res.text()); return; }
  postInput.value=''; setError(postError,''); loadPostCategories();
}
async function editVideoCategory(id,current){
  const name=window.prompt('新的分类名称', current||''); if(!name) return;
  const res=await fetch('/api/video-categories',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id,name:name.trim()})});
  if(!res.ok){ setError(videoError,await res.text()); return; }
  loadVideoCategories();
}
async function editPostCategory(id,current){
  const name=window.prompt('新的分类名称', current||''); if(!name) return;
  const res=await fetch('/api/post-categories',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id,name:name.trim()})});
  if(!res.ok){ setError(postError,await res.text()); return; }
  loadPostCategories();
}
async function deleteVideoCategory(id){
  const res=await fetch('/api/video-categories',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id})});
  if(!res.ok){ setError(videoError,await res.text()); return; }
  loadVideoCategories();
}
async function deletePostCategory(id){
  const res=await fetch('/api/post-categories',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id})});
  if(!res.ok){ setError(postError,await res.text()); return; }
  loadPostCategories();
}

async function loadNotifications(){
  try{
    const res=await fetch('/api/system-notifications'); const list=await res.json(); notifListEl.innerHTML='';
    if(!list||list.length===0){ notifListEl.innerHTML='<div style="color:#999;font-size:13px;">暂无系统通知</div>'; return; }
    list.forEach(function(item){
      const row=document.createElement('div'); row.className='notif-item';
      row.innerHTML='<div class="notif-info"><div class="notif-title">'+escapeHtml(item.title)+'</div><div class="notif-content">'+escapeHtml(item.content)+'</div><div class="notif-time">'+escapeHtml(item.createdAt)+'</div></div>';
      const btn=document.createElement('button'); btn.textContent='删除'; btn.onclick=function(){ deleteNotification(item.id); }; row.appendChild(btn); notifListEl.appendChild(row);
    });
  }catch(e){ setError(notifError,'加载失败'); }
}
async function addNotification(){
  const title=(notifTitleInput.value||'').trim(); const content=(notifContentInput.value||'').trim();
  if(!title||!content){ setError(notifError,'请输入标题和内容'); return; }
  const res=await fetch('/api/system-notifications',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({title:title,content:content})});
  if(!res.ok){ setError(notifError,await res.text()); return; }
  notifTitleInput.value=''; notifContentInput.value=''; setError(notifError,''); loadNotifications();
}
async function deleteNotification(id){
  if(!window.confirm('确认删除这条通知吗？')) return;
  const res=await fetch('/api/system-notifications',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id})});
  if(!res.ok){ setError(notifError,await res.text()); return; }
  loadNotifications();
}

function renderManualReviews(list){
  manualReviewListEl.innerHTML='';
  if(!list || list.length===0){
    manualReviewListEl.innerHTML='<div style="color:#999;font-size:13px;">暂无待处理复审请求</div>';
    return;
  }
  list.forEach(function(item){
    const row=document.createElement('div');
    row.className='review-item';
    row.innerHTML = '<div class="review-main">'+
      '<div class="review-title">'+escapeHtml(item.title || '(无标题视频)')+'</div>'+
      '<div class="review-meta">视频ID：'+escapeHtml(item.videoId || '')+'</div>'+
      '<div class="review-meta">申请人：'+escapeHtml(item.requesterEmail || '')+'</div>'+
      '<div class="review-meta">申请时间：'+escapeHtml(item.createdAt || '')+'</div>'+
    '</div>';
    const actions=document.createElement('div');
    actions.className='review-actions';
    const watchBtn=document.createElement('a');
    watchBtn.className='review-watch';
    watchBtn.textContent='查看视频';
    watchBtn.target='_blank';
    watchBtn.rel='noopener noreferrer';
    watchBtn.href=item.watchUrl || ('http://localhost:8080/player.html?id='+encodeURIComponent(item.videoId||''));
    const passBtn=document.createElement('button');
    passBtn.className='review-pass';
    passBtn.textContent='√';
    passBtn.title='复审通过';
    passBtn.onclick=function(){ handleManualReview(item.id,'approve'); };
    const rejectBtn=document.createElement('button');
    rejectBtn.className='review-reject';
    rejectBtn.textContent='×';
    rejectBtn.title='复审不通过并强制清除';
    rejectBtn.onclick=function(){ handleManualReview(item.id,'reject'); };
    actions.appendChild(watchBtn);
    actions.appendChild(passBtn);
    actions.appendChild(rejectBtn);
    row.appendChild(actions);
    manualReviewListEl.appendChild(row);
  });
}
async function loadManualReviews(){
  try{
    const res=await fetch('/api/manual-video-reviews');
    if(!res.ok){ setError(manualReviewError,await res.text()); return; }
    const list=await res.json();
    setError(manualReviewError,'');
    renderManualReviews(list||[]);
  }catch(e){
    setError(manualReviewError,'加载失败');
  }
}
function renderManualPostReviews(list){
  manualPostReviewListEl.innerHTML='';
  if(!list || list.length===0){
    manualPostReviewListEl.innerHTML='<div style="color:#999;font-size:13px;">暂无待处理帖子复审请求</div>';
    return;
  }
  list.forEach(function(item){
    const row=document.createElement('div');
    row.className='review-item';
    row.innerHTML = '<div class="review-main">'+
      '<div class="review-title">'+escapeHtml(item.title || '(无标题帖子)')+'</div>'+
      '<div class="review-meta">帖子ID：'+item.postId+'</div>'+
      '<div class="review-meta">申请人：'+escapeHtml(item.requesterEmail || '')+'</div>'+
      '<div class="review-meta">申请时间：'+escapeHtml(item.createdAt || '')+'</div>'+
    '</div>';
    const actions=document.createElement('div');
    actions.className='review-actions';
    const watchBtn=document.createElement('a');
    watchBtn.className='review-watch';
    watchBtn.textContent='查看帖子';
    watchBtn.target='_blank';
    watchBtn.rel='noopener noreferrer';
    watchBtn.href=item.watchUrl || ('http://localhost:8080/post_detail.html?id='+item.postId);
    const passBtn=document.createElement('button');
    passBtn.className='review-pass';
    passBtn.textContent='√';
    passBtn.title='复审通过';
    passBtn.onclick=function(){ handleManualPostReview(item.id,'approve'); };
    const rejectBtn=document.createElement('button');
    rejectBtn.className='review-reject';
    rejectBtn.textContent='×';
    rejectBtn.title='复审不通过';
    rejectBtn.onclick=function(){ handleManualPostReview(item.id,'reject'); };
    actions.appendChild(watchBtn);
    actions.appendChild(passBtn);
    actions.appendChild(rejectBtn);
    row.appendChild(actions);
    manualPostReviewListEl.appendChild(row);
  });
}
async function loadManualPostReviews(){
  try{
    const res=await fetch('/api/manual-post-reviews');
    if(!res.ok){ setError(manualPostReviewError,await res.text()); return; }
    const list=await res.json();
    setError(manualPostReviewError,'');
    renderManualPostReviews(list||[]);
  }catch(e){
    setError(manualPostReviewError,'加载失败');
  }
}
async function handleManualPostReview(id, action){
  const label = action==='approve' ? '通过复审' : '拒绝复审';
  if(!window.confirm('确认'+label+'？')) return;
  const reviewer=window.prompt('请输入审核员标识（可选）','admin') || 'admin';
  const res=await fetch('/api/manual-post-reviews',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({id:id,action:action,reviewer:reviewer})
  });
  if(!res.ok){ setError(manualPostReviewError,await res.text()); return; }
  setError(manualPostReviewError,'');
  loadManualPostReviews();
}
async function handleManualReview(id, action){
  const label = action==='approve' ? '通过复审' : '拒绝复审并强制清除视频';
  if(!window.confirm('确认'+label+'？')) return;
  const reviewer=window.prompt('请输入审核员标识（可选）','admin') || 'admin';
  const res=await fetch('/api/manual-video-reviews',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({id:id,action:action,reviewer:reviewer})
  });
  if(!res.ok){ setError(manualReviewError,await res.text()); return; }
  setError(manualReviewError,'');
  loadManualReviews();
}

function renderReportReviews(list){
  reportReviewListEl.innerHTML='';
  if(!list || list.length===0){
    reportReviewListEl.innerHTML='<div style="color:#999;font-size:13px;">暂无待处理举报复审</div>';
    return;
  }
  list.forEach(function(item){
    const row=document.createElement('div');
    row.className='review-item';
    const typeName = item.targetType==='video' ? '视频' : '帖子';
    row.innerHTML = '<div class="review-main">'+
      '<div class="review-title">['+escapeHtml(typeName)+'] '+escapeHtml(item.title || '(无标题)')+'</div>'+
      '<div class="review-meta">ID：'+escapeHtml(item.targetId || '')+'</div>'+
      '<div class="review-meta">举报数：<span style="color:#e53e3e;font-weight:600;">'+item.reportCount+'</span></div>'+
      '<div class="review-meta">进入复审时间：'+escapeHtml(item.createdAt || '')+'</div>'+
    '</div>';
    const actions=document.createElement('div');
    actions.className='review-actions';
    const watchBtn=document.createElement('a');
    watchBtn.className='review-watch';
    watchBtn.textContent='查看内容';
    watchBtn.target='_blank';
    watchBtn.rel='noopener noreferrer';
    watchBtn.href=item.watchUrl || '#';
    const keepBtn=document.createElement('button');
    keepBtn.className='review-pass';
    keepBtn.textContent='√';
    keepBtn.title='复审通过，保留内容';
    keepBtn.onclick=function(){ handleReportReview(item.id,'keep'); };
    const takedownBtn=document.createElement('button');
    takedownBtn.className='review-reject';
    takedownBtn.textContent='×';
    takedownBtn.title='下架内容（需填写原因）';
    takedownBtn.onclick=function(){ handleReportReview(item.id,'takedown'); };
    actions.appendChild(watchBtn);
    actions.appendChild(keepBtn);
    actions.appendChild(takedownBtn);
    row.appendChild(actions);
    reportReviewListEl.appendChild(row);
  });
}
async function loadReportReviews(){
  try{
    const res=await fetch('/api/report-reviews');
    if(!res.ok){ setError(reportReviewError,await res.text()); return; }
    const list=await res.json();
    setError(reportReviewError,'');
    renderReportReviews(list||[]);
  }catch(e){
    setError(reportReviewError,'加载失败');
  }
}
async function handleReportReview(id, action){
  if(action==='keep'){
    if(!window.confirm('确认复审通过，保留内容？')) return;
    const reviewer=window.prompt('请输入审核员标识（可选）','admin') || 'admin';
    const res=await fetch('/api/report-reviews',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({id:id,action:'keep',reviewer:reviewer,reason:''})
    });
    if(!res.ok){ setError(reportReviewError,await res.text()); return; }
  } else {
    const reason=window.prompt('请输入下架原因（必填）：');
    if(!reason || !reason.trim()){ alert('下架必须填写具体原因'); return; }
    const reviewer=window.prompt('请输入审核员标识（可选）','admin') || 'admin';
    if(!window.confirm('确认下架内容？原因：'+reason)) return;
    const res=await fetch('/api/report-reviews',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({id:id,action:'takedown',reviewer:reviewer,reason:reason.trim()})
    });
    if(!res.ok){ setError(reportReviewError,await res.text()); return; }
  }
  setError(reportReviewError,'');
  loadReportReviews();
}

function renderPosterList(list){
  posterListEl.innerHTML='';
  (list||[]).forEach(function(item){
    const card=document.createElement('div'); card.className='poster-item';
    card.innerHTML='<img class="poster-thumb" src="'+escapeHtml(item.imageUrl)+'" alt="poster" /><div class="poster-body"><div><strong>ID:</strong> '+item.id+'</div><div><strong>URL:</strong> '+(item.linkUrl?escapeHtml(item.linkUrl):'-')+'</div><div><strong>排序:</strong> '+item.sortOrder+'</div><div><strong>启用:</strong> '+(item.enabled?'是':'否')+'</div></div>';
    const actions=document.createElement('div'); actions.className='poster-actions';
    const editBtn=document.createElement('button'); editBtn.style.background='#00AEEC'; editBtn.textContent='编辑'; editBtn.onclick=function(){ editPoster(item); };
    const delBtn=document.createElement('button'); delBtn.textContent='删除'; delBtn.onclick=function(){ deletePoster(item.id); };
    actions.appendChild(editBtn); actions.appendChild(delBtn); card.appendChild(actions); posterListEl.appendChild(card);
  });
}
async function loadPosters(){
  try{ const res=await fetch('/api/homepage-posters'); if(!res.ok){ setError(posterError,await res.text()); return; } const list=await res.json(); setError(posterError,''); renderPosterList(list||[]); }
  catch(e){ setError(posterError,'加载失败'); }
}
async function uploadPoster(){
  const file=posterFileInput && posterFileInput.files ? posterFileInput.files[0] : null;
  if(!file){ setError(posterError,'请选择图片'); return; }
  const form=new FormData();
  form.append('file',file);
  form.append('linkUrl',(posterLinkInput.value||'').trim());
  form.append('sortOrder',String(parseInt(posterSortInput.value||'0',10)||0));
  form.append('openInNewTab',posterOpenBlankInput.checked?'true':'false');
  form.append('enabled',posterEnabledInput.checked?'true':'false');
  const res=await fetch('/api/homepage-posters/upload',{method:'POST',body:form});
  if(!res.ok){ setError(posterError,await res.text()); return; }
  posterFileInput.value=''; setError(posterError,''); loadPosters();
}
async function editPoster(item){
  const linkUrl=window.prompt('海报超链地址', item.linkUrl||''); if(linkUrl===null) return;
  const sortRaw=window.prompt('排序值', String(item.sortOrder||0)); if(sortRaw===null) return;
  const enabled=window.confirm('确定=启用，取消=停用');
  const openInNewTab=window.confirm('确定=新窗口打开，取消=当前页打开');
  const res=await fetch('/api/homepage-posters',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:item.id,linkUrl:linkUrl.trim(),sortOrder:parseInt(sortRaw,10)||0,enabled:enabled,openInNewTab:openInNewTab})});
  if(!res.ok){ setError(posterError,await res.text()); return; }
  loadPosters();
}
async function deletePoster(id){
  if(!window.confirm('确认删除海报吗？')) return;
  const res=await fetch('/api/homepage-posters',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id})});
  if(!res.ok){ setError(posterError,await res.text()); return; }
  loadPosters();
}

videoAddBtn && videoAddBtn.addEventListener('click', addVideoCategory);
postAddBtn && postAddBtn.addEventListener('click', addPostCategory);
notifAddBtn && notifAddBtn.addEventListener('click', addNotification);
posterUploadBtn && posterUploadBtn.addEventListener('click', uploadPoster);
loadVideoCategories();
loadPostCategories();
loadNotifications();
loadManualReviews();
loadManualPostReviews();
loadReportReviews();
loadPosters();

(function(){
  const batchCategorySelect = document.getElementById('batchCategorySelect');
  const batchAuthorSelect = document.getElementById('batchAuthorSelect');
  const batchAuthorEmail = document.getElementById('batchAuthorEmail');
  const batchAuthorNickname = document.getElementById('batchAuthorNickname');
  const batchFileInput = document.getElementById('batchFileInput');
  const batchUploadBtn = document.getElementById('batchUploadBtn');
  const batchError = document.getElementById('batchError');
  const batchResult = document.getElementById('batchResult');
  async function loadBatchCategories(){
    if(!batchCategorySelect) return;
    try {
      const res = await fetch('/api/video-categories');
      const list = await res.json();
      batchCategorySelect.innerHTML = '';
      const o0 = document.createElement('option'); o0.value = ''; o0.setAttribute('data-i18n-opt', 'selectCategory'); o0.textContent = (window.adminLang && window.adminLang.selectCategory) || '选择分类'; batchCategorySelect.appendChild(o0);
      (list||[]).forEach(function(c){ const o = document.createElement('option'); o.value = c.id; o.textContent = c.name || c.id; batchCategorySelect.appendChild(o); });
    } catch(e){}
  }
  async function loadBatchAuthors(){
    if(!batchAuthorSelect) return;
    try {
      const res = await fetch('/api/admin/users/list');
      const list = await res.json();
      const selOpt = (window.adminLang && window.adminLang.selectAuthor) || '选择作者';
      batchAuthorSelect.innerHTML = '<option value="">-- ' + selOpt + ' --</option>';
      (list||[]).forEach(function(u){ const o = document.createElement('option'); o.value = u.email || ''; o.textContent = (u.nickname || u.email) + ' (' + (u.email || '') + ')'; batchAuthorSelect.appendChild(o); });
    } catch(e){}
  }
  if(batchAuthorSelect) batchAuthorSelect.addEventListener('change', function(){ if(batchAuthorEmail) batchAuthorEmail.value = batchAuthorSelect.value || ''; });
  batchUploadBtn && batchUploadBtn.addEventListener('click', async function(){
    const category = (batchCategorySelect && batchCategorySelect.value) || '';
    if(!category){ setError(batchError, (window.adminLang && window.adminLang.chooseCategory) || '请选择视频分类'); return; }
    const authorEmail = ((batchAuthorSelect && batchAuthorSelect.value) || (batchAuthorEmail && batchAuthorEmail.value) || '').trim();
    if(!authorEmail){ setError(batchError, (window.adminLang && window.adminLang.authorEmailRequired) || '请选择或填写作者邮箱，视频将归属该用户'); return; }
    const files = batchFileInput && batchFileInput.files;
    if(!files || files.length === 0){ setError(batchError, (window.adminLang && window.adminLang.chooseFiles) || '请选择至少一个视频文件'); return; }
    setError(batchError,'');
    const total = files.length;
    const allUploaded = [];
    for(let i = 0; i < total; i++){
      batchResult.textContent = '上传中 ' + (i + 1) + '/' + total + '...';
      const form = new FormData();
      form.append('category', category);
      form.append('authorEmail', authorEmail);
      form.append('authorNickname', (batchAuthorNickname && batchAuthorNickname.value) || '');
      form.append('files', files[i]);
      try {
        const res = await fetch('/api/admin/batch-upload', { method: 'POST', body: form });
        const text = await res.text();
        let data = {};
        try { data = JSON.parse(text); } catch(e) {}
        if(!res.ok){ setError(batchError, data.error || data.message || text || '上传失败'); batchResult.textContent = ''; return; }
        if(data.uploaded && data.uploaded.length) allUploaded.push.apply(allUploaded, data.uploaded);
      } catch(e){ setError(batchError, '上传失败：' + (e && e.message ? e.message : String(e))); batchResult.textContent = ''; return; }
    }
    setError(batchError,'');
    batchResult.textContent = '成功上传 ' + allUploaded.length + ' 个视频：' + allUploaded.join(', ');
    batchFileInput.value = '';
  });
  loadBatchCategories();
  loadBatchAuthors();

  const createUserEmail = document.getElementById('createUserEmail');
  const createUserNickname = document.getElementById('createUserNickname');
  const createUserPassword = document.getElementById('createUserPassword');
  const createUserBtn = document.getElementById('createUserBtn');
  const createUserError = document.getElementById('createUserError');
  createUserBtn && createUserBtn.addEventListener('click', async function(){
    const email = (createUserEmail && createUserEmail.value) ? createUserEmail.value.trim() : '';
    const nickname = (createUserNickname && createUserNickname.value) ? createUserNickname.value.trim() : '';
    const password = (createUserPassword && createUserPassword.value) ? createUserPassword.value : '';
    if(!email){ setError(createUserError, '请填写邮箱'); return; }
    if(!password){ setError(createUserError, '请填写密码（6位数字）'); return; }
    setError(createUserError, '');
    try {
      const res = await fetch('/api/admin/users/create', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email, nickname: nickname, password: password }) });
      const data = await res.json().catch(function(){ return {}; });
      if(!res.ok){ setError(createUserError, data.error || await res.text()); return; }
      setError(createUserError, ''); if(createUserEmail) createUserEmail.value = ''; if(createUserNickname) createUserNickname.value = ''; if(createUserPassword) createUserPassword.value = ''; loadBatchAuthors(); alert('用户已创建');
    } catch(e){ setError(createUserError, '请求失败'); }
  });

  const takedownSearchInput = document.getElementById('takedownSearchInput');
  const takedownSearchVideosBtn = document.getElementById('takedownSearchVideosBtn');
  const takedownSearchPostsBtn = document.getElementById('takedownSearchPostsBtn');
  const takedownError = document.getElementById('takedownError');
  const takedownVideoList = document.getElementById('takedownVideoList');
  const takedownPostList = document.getElementById('takedownPostList');
  function renderTakedownVideos(list){
    if(!takedownVideoList) return;
    takedownVideoList.innerHTML = '';
    (list||[]).forEach(function(item){
      const row = document.createElement('div'); row.className = 'review-item';
      row.innerHTML = '<div class="review-main"><div class="review-title">' + escapeHtml(item.title || item.videoId) + '</div><div class="review-meta">ID: ' + escapeHtml(item.videoId) + ' | 状态: ' + escapeHtml(item.reviewStatus) + (item.takedownReason ? ' | 下架原因: ' + escapeHtml(item.takedownReason) : '') + '</div></div>';
      const btn = document.createElement('button'); btn.className = 'review-reject'; btn.textContent = '下架'; btn.onclick = function(){ const reason = window.prompt('请输入下架原因（必填）：'); if(!reason || !reason.trim()){ alert('必须填写具体原因'); return; } doTakedownVideo(item.videoId, reason); };
      row.appendChild(btn); takedownVideoList.appendChild(row);
    });
  }
  function renderTakedownPosts(list){
    if(!takedownPostList) return;
    takedownPostList.innerHTML = '';
    (list||[]).forEach(function(item){
      const row = document.createElement('div'); row.className = 'review-item';
      row.innerHTML = '<div class="review-main"><div class="review-title">' + escapeHtml(item.title) + '</div><div class="review-meta">ID: ' + item.postId + ' | ' + escapeHtml(item.nickname || item.email) + ' | 状态: ' + escapeHtml(item.reviewStatus) + (item.takedownReason ? ' | 下架原因: ' + escapeHtml(item.takedownReason) : '') + '</div></div>';
      const btn = document.createElement('button'); btn.className = 'review-reject'; btn.textContent = '下架'; btn.onclick = function(){ const reason = window.prompt('请输入下架原因（必填）：'); if(!reason || !reason.trim()){ alert('必须填写具体原因'); return; } doTakedownPost(item.postId, reason); };
      row.appendChild(btn); takedownPostList.appendChild(row);
    });
  }
  async function doTakedownVideo(videoId, reason){
    const res = await fetch('/api/admin/takedown/video', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ videoId: videoId, reason: reason }) });
    if(!res.ok){ setError(takedownError, await res.text()); return; } setError(takedownError,''); takedownSearchVideosBtn.click();
  }
  async function doTakedownPost(postId, reason){
    const res = await fetch('/api/admin/takedown/post', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ postId: postId, reason: reason }) });
    if(!res.ok){ setError(takedownError, await res.text()); return; } setError(takedownError,''); takedownSearchPostsBtn.click();
  }
  takedownSearchVideosBtn && takedownSearchVideosBtn.addEventListener('click', async function(){
    const q = (takedownSearchInput && takedownSearchInput.value) || '';
    try { const res = await fetch('/api/admin/search/videos?q=' + encodeURIComponent(q)); const list = await res.json(); setError(takedownError,''); renderTakedownVideos(list); } catch(e){ setError(takedownError, '加载失败'); }
  });
  takedownSearchPostsBtn && takedownSearchPostsBtn.addEventListener('click', async function(){
    const q = (takedownSearchInput && takedownSearchInput.value) || '';
    try { const res = await fetch('/api/admin/search/posts?q=' + encodeURIComponent(q)); const list = await res.json(); setError(takedownError,''); renderTakedownPosts(list); } catch(e){ setError(takedownError, '加载失败'); }
  });

  const userSearchInput = document.getElementById('userSearchInput');
  const userSearchBtn = document.getElementById('userSearchBtn');
  const userManageError = document.getElementById('userManageError');
  const userList = document.getElementById('userList');
  userSearchBtn && userSearchBtn.addEventListener('click', async function(){
    const q = (userSearchInput && userSearchInput.value) || '';
    try {
      const res = await fetch('/api/admin/search/users?q=' + encodeURIComponent(q));
      const list = await res.json();
      setError(userManageError,'');
      userList.innerHTML = '';
      (list||[]).forEach(function(u){
        const row = document.createElement('div'); row.className = 'review-item';
        row.innerHTML = '<div class="review-main"><div class="review-title">' + escapeHtml(u.nickname || u.email) + '</div><div class="review-meta">' + escapeHtml(u.email) + '</div></div>';
        const actions = document.createElement('div'); actions.className = 'review-actions';
        const banBtn = document.createElement('button'); banBtn.className = 'review-reject'; banBtn.textContent = '封号'; banBtn.onclick = function(){ const reason = window.prompt('封号原因（必填）：'); if(!reason || !reason.trim()) return; const until = window.prompt('封禁截止时间（如 2025-03-08 00:00:00 或 2025-03-08）：'); if(!until || !until.trim()) return; doBan(u.email, reason, until); };
        const muteBtn = document.createElement('button'); muteBtn.style.background = '#f59e0b'; muteBtn.style.border = 'none'; muteBtn.style.color = '#fff'; muteBtn.style.padding = '4px 10px'; muteBtn.style.borderRadius = '4px'; muteBtn.textContent = '禁言'; muteBtn.onclick = function(){ const reason = window.prompt('禁言原因（必填）：'); if(!reason || !reason.trim()) return; const until = window.prompt('禁言截止时间（如 2025-03-08 00:00:00）：'); if(!until || !until.trim()) return; doMute(u.email, reason, until); };
        const pardonBtn = document.createElement('button'); pardonBtn.className = 'review-pass'; pardonBtn.textContent = '赦免封号'; pardonBtn.onclick = function(){ const reason = window.prompt('赦免原因（必填）：'); if(!reason || !reason.trim()) return; doPardon(u.email, reason); };
        actions.appendChild(banBtn); actions.appendChild(muteBtn); actions.appendChild(pardonBtn); row.appendChild(actions); userList.appendChild(row);
      });
    } catch(e){ setError(userManageError, '加载失败'); }
  });
  async function doBan(email, reason, bannedUntil){ const res = await fetch('/api/admin/users/ban', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email, reason: reason, bannedUntil: bannedUntil }) }); if(!res.ok) setError(userManageError, await res.text()); else setError(userManageError,''); }
  async function doMute(email, reason, mutedUntil){ const res = await fetch('/api/admin/users/mute', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email, reason: reason, mutedUntil: mutedUntil }) }); if(!res.ok) setError(userManageError, await res.text()); else setError(userManageError,''); }
  async function doPardon(email, reason){ const res = await fetch('/api/admin/users/pardon', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email, reason: reason }) }); if(!res.ok) setError(userManageError, await res.text()); else setError(userManageError,''); }

  (function adminLang(){
    window.adminLang = {};
    const sel = document.getElementById('adminLangSelect');
    if(sel){ sel.value = localStorage.getItem('evw_admin_locale') || 'zh-CN'; }
    function apply(lang){
      if(!lang) return;
      window.adminLang = lang;
      if(lang.title) document.title = lang.title;
      document.querySelectorAll('[data-i18n]').forEach(function(el){ const k = el.getAttribute('data-i18n'); if(lang[k]) el.textContent = lang[k]; });
      document.querySelectorAll('[data-i18n-placeholder]').forEach(function(el){ const k = el.getAttribute('data-i18n-placeholder'); if(lang[k]) el.placeholder = lang[k]; });
      document.querySelectorAll('[data-i18n-opt]').forEach(function(el){ const k = el.getAttribute('data-i18n-opt'); if(lang[k]) el.textContent = lang[k]; });
    }
    function load(){
      const locale = (sel && sel.value) || 'zh-CN';
      localStorage.setItem('evw_admin_locale', locale);
      fetch('/language/admin-' + locale + '.json').then(function(r){ return r.ok ? r.json() : {}; }).then(apply);
    }
    if(sel) sel.addEventListener('change', load);
    load();
  })();
})();
</script>
</body>
</html>`

func handleAdminLangFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/language/")
	if name == "" || strings.Contains(name, "..") || !strings.HasPrefix(name, "admin-") {
		http.NotFound(w, r)
		return
	}
	for _, base := range []string{"web/language", "../web/language"} {
		p := filepath.Join(base, name)
		data, err := os.ReadFile(p)
		if err == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Write(data)
			return
		}
	}
	http.NotFound(w, r)
}

func main() {
	if err := loadMySQLConfig("mysql.local.json"); err != nil {
		panic(err)
	}
	_ = os.MkdirAll(filepath.Join("storage", "posters"), 0o755)
	initMySQL()
	mux := http.NewServeMux()
	// 静态资源（如首页海报图片）从 storage 目录通过 /media/ 暴露，避免管理端看不到图片
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir("storage"))))
	// 先注册所有 API 和 /language/，最后注册 "/" 避免抢掉 /api/* 请求
	mux.HandleFunc("/language/", handleAdminLangFile)
	mux.HandleFunc("/api/video-categories", handleAdminVideoCategories)
	mux.HandleFunc("/api/post-categories", handleAdminPostCategories)
	mux.HandleFunc("/api/system-notifications", handleAdminSystemNotifications)
	mux.HandleFunc("/api/manual-video-reviews", handleAdminManualVideoReviews)
	mux.HandleFunc("/api/manual-post-reviews", handleAdminManualPostReviews)
	mux.HandleFunc("/api/report-reviews", handleAdminReportReviews)
	mux.HandleFunc("/api/homepage-posters", handleAdminHomepagePosters)
	mux.HandleFunc("/api/homepage-posters/upload", handleAdminHomepagePosterUpload)
	mux.HandleFunc("/api/admin/batch-upload", handleAdminBatchUpload)
	mux.HandleFunc("/api/admin/search/videos", handleAdminSearchVideos)
	mux.HandleFunc("/api/admin/search/posts", handleAdminSearchPosts)
	mux.HandleFunc("/api/admin/search/users", handleAdminSearchUsers)
	mux.HandleFunc("/api/admin/users/list", handleAdminUsersList)
	mux.HandleFunc("/api/admin/users/create", handleAdminUsersCreate)
	mux.HandleFunc("/api/admin/takedown/video", handleAdminTakedownVideo)
	mux.HandleFunc("/api/admin/takedown/post", handleAdminTakedownPost)
	mux.HandleFunc("/api/admin/users/ban", handleAdminUserBan)
	mux.HandleFunc("/api/admin/users/mute", handleAdminUserMute)
	mux.HandleFunc("/api/admin/users/pardon", handleAdminUserPardon)
	mux.HandleFunc("/", handleAdminPage)
	server := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}
	fmt.Println("http://localhost:8081")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
