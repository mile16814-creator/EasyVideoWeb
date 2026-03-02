package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FormatInfo struct {
	Container   string  `json:"container"`
	VideoCodec  string  `json:"videoCodec"`
	AudioCodec  string  `json:"audioCodec"`
	DurationSec float64 `json:"durationSec"`
	SizeBytes   int64   `json:"sizeBytes"`
	Bitrate     int64   `json:"bitrate"`
}

type Video struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Category       string     `json:"category"`
	Tags           []string   `json:"tags"`
	ThumbURL       string     `json:"thumbUrl"`
	PlayURL        string     `json:"playUrl"`
	Views          int        `json:"views"`
	DurationSec    float64    `json:"durationSec"`
	Format         FormatInfo `json:"format"`
	CreatedAt      time.Time  `json:"createdAt"`
	AuthorNickname string     `json:"authorNickname"`
	AuthorEmail    string     `json:"authorEmail"`
	Description    string     `json:"description"`
	LikeCount      int        `json:"likeCount"`
	CommentCount   int        `json:"commentCount"`
	Score          float64    `json:"score"`
	FavoriteAt     time.Time  `json:"favoriteAt,omitempty"`
	ReviewStatus   string     `json:"reviewStatus,omitempty"`
	TakedownReason string     `json:"takedownReason,omitempty"`
}

type Post struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	Nickname     string    `json:"nickname"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	ImagePath    string    `json:"imagePath"`
	Category     string    `json:"category"`
	CreatedAt    time.Time `json:"createdAt"`
	LikeCount    int       `json:"likeCount"`
	Views        int       `json:"views"`
	AvatarURL    string    `json:"avatarUrl"` // Joined field
	FavoriteAt   time.Time `json:"favoriteAt,omitempty"`
	ReviewStatus   string `json:"reviewStatus,omitempty"`
	TakedownReason string `json:"takedownReason,omitempty"`
}

type HomepagePoster struct {
	ID           int64     `json:"id"`
	ImageURL     string    `json:"imageUrl"`
	LinkURL      string    `json:"linkUrl"`
	OpenInNewTab bool      `json:"openInNewTab"`
	Enabled      bool      `json:"enabled"`
	SortOrder    int       `json:"sortOrder"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type probeOutput struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecName string `json:"codec_name"`
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

var (
	supportedExts         = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true}
	likesPerExtraPublish  = 20
	baseVideoPublishLimit = 3
	basePostPublishLimit  = 3
	scanInterval          = 2 * time.Second
	lastScan              time.Time
	scanInProgress        bool
	scanMtx               sync.Mutex

	verificationCodes = make(map[string]VerificationCode)
	verificationMtx   sync.Mutex
	smtpHost          = "smtp.gmail.com"
	smtpPort          = "587"
	smtpUser          = ""
	smtpPass          = ""

	jwtSecret []byte
	db        *sql.DB
	mysqlCfg  mysqlConfig

	reviewSem         = make(chan struct{}, 2)
	reviewActiveCount int64
	reviewWaitCount   int64

	videoRankingCache []Video
	postRankingCache  []Post
	rankingMux        sync.RWMutex
)

type VerificationCode struct {
	Email      string
	Code       string
	ExpiresAt  time.Time
	Purpose    string
	OwnerEmail string
}

type User struct {
	Email        string    `json:"email"`
	Nickname     string    `json:"nickname"`
	PasswordHash string    `json:"passwordHash"`
	PasswordSalt string    `json:"passwordSalt"`
	CreatedAt    time.Time `json:"createdAt"`
	Balance      float64   `json:"balance"`
	AvatarURL    string    `json:"avatarUrl"`
	BannerURL    string    `json:"bannerUrl"`
	Notice       string    `json:"notice"`
	Motto        string    `json:"motto"`
}

func main() {
	var debugMode bool
	flag.BoolVar(&debugMode, "debug", false, "enable debug logging")
	flag.Parse()
	if !debugMode {
		log.SetOutput(io.Discard)
	}
	loadSMTPConfig("smtp.local.json")
	if err := loadMySQLConfig("mysql.local.json"); err != nil {
		fmt.Fprintln(os.Stderr, "MySQL 配置错误:", err)
		fmt.Fprintln(os.Stderr, "请正确配置 mysql.local.json（参考 mysql.local.example.json）后重试。")
		fmt.Print("按回车键退出...")
		bufio.NewScanner(os.Stdin).Scan()
		os.Exit(1)
	}
	loadJWTSecret("jwt.local.json")
	err := os.MkdirAll(filepath.Join("storage", "videos"), 0o755)
	if err != nil {
		panic(err)
	}
	_ = os.MkdirAll(filepath.Join("storage", "avatars"), 0o755)
	_ = os.MkdirAll(filepath.Join("storage", "banners"), 0o755)
	_ = os.MkdirAll(filepath.Join("storage", "posters"), 0o755)
	ensurePythonDeps()
	initMySQL()
	loadAppConfig()
	refreshFromStorage()
	refreshRankings()
	go runRankingsRefreshEvery12h()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("web")))
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir("storage"))))
	mux.Handle("/media/avatars/", http.StripPrefix("/media/avatars/", http.FileServer(http.Dir("storage/avatars"))))
	mux.Handle("/media/banners/", http.StripPrefix("/media/banners/", http.FileServer(http.Dir("storage/banners"))))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))
	mux.HandleFunc("/api/categories", handleCategories)
	mux.HandleFunc("/api/video-categories", handleAdminVideoCategoriesAPI)
	mux.HandleFunc("/api/post-categories", handlePostCategories)
	mux.HandleFunc("/api/videos", handleVideos)
	mux.HandleFunc("/api/posts", handlePosts)
	mux.HandleFunc("/api/post-images", handlePostImageUpload)
	mux.HandleFunc("/api/posts/", handlePostDetail)
	mux.HandleFunc("/api/videos/", handleVideoDetail)
	mux.HandleFunc("/api/videos/favorite", handleToggleFavorite)
	mux.HandleFunc("/api/videos/like", handleToggleVideoLike)
	mux.HandleFunc("/api/videos/view", handleVideoView)
	mux.HandleFunc("/api/posts/favorite", handleTogglePostFavorite)
	mux.HandleFunc("/api/posts/like", handleTogglePostLike)
	mux.HandleFunc("/api/posts/view", handlePostView)
	mux.HandleFunc("/api/comments/", handleCommentLike)
	mux.HandleFunc("/api/post-comments/", handlePostCommentLike)
	mux.HandleFunc("/api/send-code", handleSendCode)
	mux.HandleFunc("/api/verify-code", handleVerifyCode)
	mux.HandleFunc("/api/login", handlePasswordLogin)
	mux.HandleFunc("/api/login-code/send", handleSendLoginCode)
	mux.HandleFunc("/api/login-code/verify", handleVerifyLoginCode)
	mux.HandleFunc("/api/profile", handleProfile)
	mux.HandleFunc("/api/users/profile", handlePublicProfile)
	mux.HandleFunc("/api/profile/nickname", handleUpdateNickname)
	mux.HandleFunc("/api/profile/password", handleUpdatePassword)
	mux.HandleFunc("/api/profile/avatar", handleUpdateAvatar)
	mux.HandleFunc("/api/profile/banner", handleUpdateBanner)
	mux.HandleFunc("/api/profile/notice", handleUpdateNotice)
	mux.HandleFunc("/api/profile/motto", handleUpdateMotto)
	mux.HandleFunc("/api/profile/favorites", handleUserFavorites)
	mux.HandleFunc("/api/profile/post-favorites", handleUserPostFavorites)
	mux.HandleFunc("/api/creator/upload", handleCreatorUpload)
	mux.HandleFunc("/api/creator/publish-quota", handleCreatorPublishQuota)
	mux.HandleFunc("/api/change-email/send", handleSendChangeEmailCode)
	mux.HandleFunc("/api/change-email/verify", handleVerifyChangeEmail)
	mux.HandleFunc("/api/messages", handleMessages)
	mux.HandleFunc("/api/messages/read", handleMessagesRead)
	mux.HandleFunc("/api/messages/unread-count", handleMessagesUnreadCount)
	mux.HandleFunc("/api/messages/delete", handleMessagesDelete)
	mux.HandleFunc("/api/messages/delete-all", handleMessagesDeleteAll)
	mux.HandleFunc("/api/system-notifications", handleSystemNotifications)
	mux.HandleFunc("/api/homepage-posters", handleHomepagePosters)
	mux.HandleFunc("/api/rankings/videos", handleRankingsVideos)
	mux.HandleFunc("/api/rankings/posts", handleRankingsPosts)
	mux.HandleFunc("/api/review/queue", handleReviewQueue)
	mux.HandleFunc("/api/user-punishment", handleUserPunishment)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Println("http://localhost:8080")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func handleCategories(w http.ResponseWriter, r *http.Request) {
	refreshFromStorage()
	rows, err := db.Query("SELECT id, name FROM video_categories ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	result := []Category{{ID: "all", Name: "全部"}}
	for rows.Next() {
		var item Category
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		result = append(result, item)
	}
	writeJSON(w, result)
}

// handleAdminVideoCategoriesAPI 供管理后台使用的视频分类 CRUD，与 admin 端一致，保证单端口时后台能加载数据
func handleAdminVideoCategoriesAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name FROM video_categories ORDER BY created_at DESC")
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
			fp := filepath.Join("storage", "videos", name, filename)
			if _, err := db.Exec("UPDATE video_library SET category = ?, play_url = ?, thumb_url = ?, file_path = ?, updated_at = ? WHERE video_id = ?",
				name, playURL, thumbURL, fp, time.Now(), videoID); err != nil {
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

func handlePostCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name FROM post_categories ORDER BY created_at DESC")
		if err != nil {
			http.Error(w, "failed to load categories", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		result := []Category{{ID: "all", Name: "全部"}}
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
		if err := db.QueryRow("SELECT 1 FROM post_categories WHERE id = ?", name).Scan(&exists); err == nil {
			writeJSON(w, map[string]any{"status": "ok"})
			return
		} else if err != sql.ErrNoRows {
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

func handleVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	refreshFromStorage()
	category := r.URL.Query().Get("category")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	author := strings.TrimSpace(r.URL.Query().Get("author"))
	list, err := queryVideosFromDB(category, query, author)
	if err != nil {
		http.Error(w, "failed to load videos", http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

func handleVideoDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/videos/")
	if strings.HasSuffix(path, "/comments") {
		id := strings.TrimSuffix(path, "/comments")
		id = strings.TrimSuffix(id, "/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		handleVideoComments(w, r, id)
		return
	}
	if strings.HasSuffix(path, "/manual-review") {
		id := strings.TrimSuffix(path, "/manual-review")
		id = strings.TrimSuffix(id, "/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		handleRequestManualReview(w, r, id)
		return
	}
	id := path
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodDelete {
		handleDeleteVideo(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	refreshFromStorage()
	video, ok := findVideo(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var takedownReasonForAuthor string
	if video.ReviewStatus == "takedown" {
		var reason string
		var authorEmail string
		_ = db.QueryRow("SELECT COALESCE(takedown_reason, ''), COALESCE(author_email, '') FROM video_library WHERE video_id = ?", id).Scan(&reason, &authorEmail)
		if authorEmail == "" {
			_ = db.QueryRow("SELECT email FROM video_uploads WHERE video_id = ?", id).Scan(&authorEmail)
		}
		reqUser, authed := getAuthUserOptional(r)
		allowed := authed && reqUser.Email == authorEmail
		if !allowed {
			adminToken := strings.TrimSpace(r.URL.Query().Get("adminReviewToken"))
			if adminToken == "" {
				adminToken = strings.TrimSpace(r.Header.Get("X-Admin-Review-Token"))
			}
			if canAdminReviewAccessVideo(id, adminToken) {
				allowed = true
			}
		}
		if !allowed {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "takedown", "reason": reason})
			return
		}
		takedownReasonForAuthor = reason
	}
	if video.ReviewStatus != "" && video.ReviewStatus != "approved" && video.ReviewStatus != "takedown" {
		reqUser, authed := getAuthUserOptional(r)
		allowed := false
		if authed {
			allowed = reqUser.Email == video.AuthorEmail
			if !allowed {
				var ownerEmail string
				if err := db.QueryRow("SELECT email FROM video_uploads WHERE video_id = ?", id).Scan(&ownerEmail); err == nil && ownerEmail == reqUser.Email {
					allowed = true
				}
			}
		}
		if !allowed {
			adminToken := strings.TrimSpace(r.URL.Query().Get("adminReviewToken"))
			if adminToken == "" {
				adminToken = strings.TrimSpace(r.Header.Get("X-Admin-Review-Token"))
			}
			if canAdminReviewAccessVideo(id, adminToken) {
				allowed = true
			}
		}
		if !allowed {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}

	isFavorite := false
	isLiked := false
	if user, ok := getAuthUserOptional(r); ok {
		var exists int
		err := db.QueryRow("SELECT 1 FROM video_favorites WHERE video_id = ? AND email = ?", id, user.Email).Scan(&exists)
		if err == nil {
			isFavorite = true
		}
		err = db.QueryRow("SELECT 1 FROM video_likes WHERE video_id = ? AND email = ?", id, user.Email).Scan(&exists)
		if err == nil {
			isLiked = true
		}
	}

	var likeCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM video_likes WHERE video_id = ?", id).Scan(&likeCount)
	var viewCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM video_views WHERE video_id = ?", id).Scan(&viewCount)

	type videoDetail struct {
		Video
		IsFavorite      bool   `json:"isFavorite"`
		IsLiked         bool   `json:"isLiked"`
		LikeCount       int    `json:"likeCount"`
		AuthorAvatarURL string `json:"authorAvatarUrl"`
		AuthorMotto     string `json:"authorMotto"`
	}

	var authorAvatar string
	var authorMotto string
	if video.AuthorEmail != "" {
		var avatar, motto sql.NullString
		_ = db.QueryRow("SELECT avatar_url, motto FROM users WHERE email = ?", video.AuthorEmail).Scan(&avatar, &motto)
		if avatar.Valid {
			authorAvatar = avatar.String
		}
		if motto.Valid {
			authorMotto = motto.String
		}
	}

	writeJSON(w, videoDetail{
		Video: Video{
			ID:             video.ID,
			Title:          video.Title,
			Category:       video.Category,
			Tags:           video.Tags,
			ThumbURL:       video.ThumbURL,
			PlayURL:        video.PlayURL,
			Views:          viewCount,
			DurationSec:    video.DurationSec,
			Format:         video.Format,
			CreatedAt:      video.CreatedAt,
			AuthorNickname: video.AuthorNickname,
			AuthorEmail:    video.AuthorEmail,
			Description:    video.Description,
			LikeCount:      video.LikeCount,
			CommentCount:   video.CommentCount,
			Score:          video.Score,
			ReviewStatus:   video.ReviewStatus,
			TakedownReason: takedownReasonForAuthor,
		},
		IsFavorite:      isFavorite,
		IsLiked:         isLiked,
		LikeCount:       likeCount,
		AuthorAvatarURL: authorAvatar,
		AuthorMotto:     authorMotto,
	})
}

func handleVideoComments(w http.ResponseWriter, r *http.Request, videoID string) {
	refreshFromStorage()
	_, ok := findVideo(videoID)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		userEmail := ""
		if user, ok := getAuthUserOptional(r); ok {
			userEmail = user.Email
		}
		rows, err := db.Query(`SELECT c.id, c.nickname, c.email, c.content, c.created_at, c.parent_id,
			COALESCE(c.review_status, 'approved') AS review_status,
			cu.avatar_url AS avatar_url,
			p.nickname AS parent_nickname, p.content AS parent_content, p.email AS parent_email,
			pu.avatar_url AS parent_avatar_url,
			COALESCE(lc.cnt, 0) AS like_count,
			COALESCE(ul.liked, 0) AS liked
			FROM video_comments c
			LEFT JOIN users cu ON cu.email = c.email
			LEFT JOIN video_comments p ON c.parent_id = p.id
			LEFT JOIN users pu ON pu.email = p.email
			LEFT JOIN (
				SELECT comment_id, COUNT(*) AS cnt
				FROM video_comment_likes
				GROUP BY comment_id
			) lc ON lc.comment_id = c.id
			LEFT JOIN (
				SELECT comment_id, 1 AS liked
				FROM video_comment_likes
				WHERE email = ?
			) ul ON ul.comment_id = c.id
			WHERE c.video_id = ?
			AND (COALESCE(c.review_status, 'approved') = 'approved' OR c.email = ?)
			ORDER BY c.created_at DESC
			LIMIT 100`, userEmail, videoID, userEmail)
		if err != nil {
			http.Error(w, "failed to load comments", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type commentItem struct {
			ID             int64     `json:"id"`
			Nickname       string    `json:"nickname"`
			Email          string    `json:"email"`
			Content        string    `json:"content"`
			CreatedAt      time.Time `json:"createdAt"`
			ParentID       int64     `json:"parentId"`
			ReviewStatus   string    `json:"reviewStatus,omitempty"`
			AvatarURL      string    `json:"avatarUrl"`
			ParentNickname string    `json:"parentNickname"`
			ParentContent  string    `json:"parentContent"`
			ParentEmail    string    `json:"parentEmail"`
			ParentAvatar   string    `json:"parentAvatarUrl"`
			LikeCount      int       `json:"likeCount"`
			Liked          bool      `json:"liked"`
		}
		comments := make([]commentItem, 0)
		for rows.Next() {
			var item commentItem
			var likedInt int
			var parentID sql.NullInt64
			var avatarURL, parentNickname, parentContent, parentEmail, parentAvatarURL sql.NullString
			if err := rows.Scan(&item.ID, &item.Nickname, &item.Email, &item.Content, &item.CreatedAt, &parentID, &item.ReviewStatus, &avatarURL, &parentNickname, &parentContent, &parentEmail, &parentAvatarURL, &item.LikeCount, &likedInt); err != nil {
				http.Error(w, "failed to load comments", http.StatusInternalServerError)
				return
			}
			if parentID.Valid {
				item.ParentID = parentID.Int64
			}
			if avatarURL.Valid {
				item.AvatarURL = avatarURL.String
			}
			if parentNickname.Valid {
				item.ParentNickname = parentNickname.String
			}
			if parentContent.Valid {
				item.ParentContent = parentContent.String
			}
			if parentEmail.Valid {
				item.ParentEmail = parentEmail.String
			}
			if parentAvatarURL.Valid {
				item.ParentAvatar = parentAvatarURL.String
			}
			item.Liked = likedInt == 1
			comments = append(comments, item)
		}
		writeJSON(w, comments)
	case http.MethodPost:
		user, ok := getAuthUser(w, r)
		if !ok {
			return
		}
		if reason, until, muted := getActiveMute(user.Email); muted {
			writeMuteResponse(w, reason, until)
			return
		}
		if strings.TrimSpace(user.Nickname) == "" {
			http.Error(w, "nickname required", http.StatusBadRequest)
			return
		}
		var req struct {
			Content  string `json:"content"`
			ParentID int64  `json:"parentId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		content := strings.TrimSpace(req.Content)
		if content == "" {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		if len([]rune(content)) > 500 {
			http.Error(w, "content too long", http.StatusBadRequest)
			return
		}
		var parentID sql.NullInt64
		if req.ParentID > 0 {
			var exists int
			err := db.QueryRow("SELECT 1 FROM video_comments WHERE id = ? AND video_id = ?", req.ParentID, videoID).Scan(&exists)
			if err == sql.ErrNoRows {
				http.Error(w, "parent not found", http.StatusBadRequest)
				return
			}
			if err != nil {
				http.Error(w, "failed to load parent", http.StatusInternalServerError)
				return
			}
			parentID = sql.NullInt64{Int64: req.ParentID, Valid: true}
		}
		res, err := db.Exec("INSERT INTO video_comments (video_id, email, nickname, content, created_at, parent_id, review_status) VALUES (?, ?, ?, ?, ?, ?, 'pending')",
			videoID, user.Email, user.Nickname, content, time.Now(), parentID)
		if err != nil {
			http.Error(w, "failed to save comment", http.StatusInternalServerError)
			return
		}
		commentID, _ := res.LastInsertId()
		video, ok := findVideo(videoID)
		videoTitle := ""
		if ok {
			videoTitle = video.Title
		}
		go runCommentReview("video", commentID, videoID, 0, content, user.Email, videoTitle)
		writeJSON(w, map[string]any{"status": "ok", "id": commentID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleCommentLike(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/comments/")
	if r.Method == http.MethodDelete {
		idPart := strings.TrimSuffix(path, "/")
		if idPart == "" || strings.Contains(idPart, "/") {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		commentID, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil || commentID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		user, ok := getAuthUser(w, r)
		if !ok {
			return
		}
		var ownerEmail string
		err = db.QueryRow("SELECT email FROM video_comments WHERE id = ?", commentID).Scan(&ownerEmail)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load comment", http.StatusInternalServerError)
			return
		}
		if ownerEmail != user.Email {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = db.Exec("DELETE FROM video_comment_likes WHERE comment_id = ? OR comment_id IN (SELECT id FROM video_comments WHERE parent_id = ?)", commentID, commentID)
		if _, err := db.Exec("DELETE FROM video_comments WHERE id = ? OR parent_id = ?", commentID, commentID); err != nil {
			http.Error(w, "failed to delete", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "deleted"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(path, "/like") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	idPart := strings.TrimSuffix(path, "/like")
	idPart = strings.TrimSuffix(idPart, "/")
	if idPart == "" || strings.Contains(idPart, "/") {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	commentID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || commentID <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var exists int
	err = db.QueryRow("SELECT 1 FROM video_comments WHERE id = ?", commentID).Scan(&exists)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load comment", http.StatusInternalServerError)
		return
	}
	liked := false
	err = db.QueryRow("SELECT 1 FROM video_comment_likes WHERE comment_id = ? AND email = ?", commentID, user.Email).Scan(&exists)
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO video_comment_likes (comment_id, email, created_at) VALUES (?, ?, ?)", commentID, user.Email, time.Now())
		if err != nil {
			http.Error(w, "failed to like", http.StatusInternalServerError)
			return
		}
		liked = true
	} else if err == nil {
		_, err = db.Exec("DELETE FROM video_comment_likes WHERE comment_id = ? AND email = ?", commentID, user.Email)
		if err != nil {
			http.Error(w, "failed to unlike", http.StatusInternalServerError)
			return
		}
		liked = false
	} else {
		http.Error(w, "failed to like", http.StatusInternalServerError)
		return
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM video_comment_likes WHERE comment_id = ?", commentID).Scan(&count); err != nil {
		http.Error(w, "failed to load likes", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"liked":     liked,
		"likeCount": count,
	})
}

func handlePostComments(w http.ResponseWriter, r *http.Request, postID int64) {
	switch r.Method {
	case http.MethodGet:
		userEmail := ""
		if user, ok := getAuthUserOptional(r); ok {
			userEmail = user.Email
		}
		rows, err := db.Query(`SELECT c.id, c.nickname, c.email, c.content, c.created_at, c.parent_id,
			COALESCE(c.review_status, 'approved') AS review_status,
			cu.avatar_url AS avatar_url,
			p.nickname AS parent_nickname, p.content AS parent_content, p.email AS parent_email,
			pu.avatar_url AS parent_avatar_url,
			COALESCE(lc.cnt, 0) AS like_count,
			COALESCE(ul.liked, 0) AS liked
			FROM post_comments c
			LEFT JOIN users cu ON cu.email = c.email
			LEFT JOIN post_comments p ON c.parent_id = p.id
			LEFT JOIN users pu ON pu.email = p.email
			LEFT JOIN (
				SELECT comment_id, COUNT(*) AS cnt
				FROM post_comment_likes
				GROUP BY comment_id
			) lc ON lc.comment_id = c.id
			LEFT JOIN (
				SELECT comment_id, 1 AS liked
				FROM post_comment_likes
				WHERE email = ?
			) ul ON ul.comment_id = c.id
			WHERE c.post_id = ?
			AND (COALESCE(c.review_status, 'approved') = 'approved' OR c.email = ?)
			ORDER BY c.created_at DESC
			LIMIT 100`, userEmail, postID, userEmail)
		if err != nil {
			http.Error(w, "failed to load comments", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type commentItem struct {
			ID             int64     `json:"id"`
			Nickname       string    `json:"nickname"`
			Email          string    `json:"email"`
			Content        string    `json:"content"`
			CreatedAt      time.Time `json:"createdAt"`
			ParentID       int64     `json:"parentId"`
			ReviewStatus   string    `json:"reviewStatus,omitempty"`
			AvatarURL      string    `json:"avatarUrl"`
			ParentNickname string    `json:"parentNickname"`
			ParentContent  string    `json:"parentContent"`
			ParentEmail    string    `json:"parentEmail"`
			ParentAvatar   string    `json:"parentAvatarUrl"`
			LikeCount      int       `json:"likeCount"`
			Liked          bool      `json:"liked"`
		}
		comments := make([]commentItem, 0)
		for rows.Next() {
			var item commentItem
			var likedInt int
			var parentID sql.NullInt64
			var avatarURL, parentNickname, parentContent, parentEmail, parentAvatarURL sql.NullString
			if err := rows.Scan(&item.ID, &item.Nickname, &item.Email, &item.Content, &item.CreatedAt, &parentID, &item.ReviewStatus, &avatarURL, &parentNickname, &parentContent, &parentEmail, &parentAvatarURL, &item.LikeCount, &likedInt); err != nil {
				http.Error(w, "failed to load comments", http.StatusInternalServerError)
				return
			}
			if parentID.Valid {
				item.ParentID = parentID.Int64
			}
			if avatarURL.Valid {
				item.AvatarURL = avatarURL.String
			}
			if parentNickname.Valid {
				item.ParentNickname = parentNickname.String
			}
			if parentContent.Valid {
				item.ParentContent = parentContent.String
			}
			if parentEmail.Valid {
				item.ParentEmail = parentEmail.String
			}
			if parentAvatarURL.Valid {
				item.ParentAvatar = parentAvatarURL.String
			}
			item.Liked = likedInt == 1
			comments = append(comments, item)
		}
		writeJSON(w, comments)
	case http.MethodPost:
		user, ok := getAuthUser(w, r)
		if !ok {
			return
		}
		if reason, until, muted := getActiveMute(user.Email); muted {
			writeMuteResponse(w, reason, until)
			return
		}
		if strings.TrimSpace(user.Nickname) == "" {
			http.Error(w, "nickname required", http.StatusBadRequest)
			return
		}
		var req struct {
			Content  string `json:"content"`
			ParentID int64  `json:"parentId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		content := strings.TrimSpace(req.Content)
		if content == "" {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		if len([]rune(content)) > 500 {
			http.Error(w, "content too long", http.StatusBadRequest)
			return
		}
		var postTitle string
		if err := db.QueryRow("SELECT title FROM posts WHERE id = ?", postID).Scan(&postTitle); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "post not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to load post", http.StatusInternalServerError)
			return
		}
		var parentID sql.NullInt64
		if req.ParentID > 0 {
			var exists int
			err := db.QueryRow("SELECT 1 FROM post_comments WHERE id = ? AND post_id = ?", req.ParentID, postID).Scan(&exists)
			if err == sql.ErrNoRows {
				http.Error(w, "parent not found", http.StatusBadRequest)
				return
			}
			if err != nil {
				http.Error(w, "failed to load parent", http.StatusInternalServerError)
				return
			}
			parentID = sql.NullInt64{Int64: req.ParentID, Valid: true}
		}
		res, err := db.Exec("INSERT INTO post_comments (post_id, email, nickname, content, created_at, parent_id, review_status) VALUES (?, ?, ?, ?, ?, ?, 'pending')",
			postID, user.Email, user.Nickname, content, time.Now(), parentID)
		if err != nil {
			http.Error(w, "failed to save comment", http.StatusInternalServerError)
			return
		}
		commentID, _ := res.LastInsertId()
		go runCommentReview("post", commentID, "", postID, content, user.Email, postTitle)
		writeJSON(w, map[string]any{"status": "ok", "id": commentID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePostCommentLike(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/post-comments/")
	if r.Method == http.MethodDelete {
		idPart := strings.TrimSuffix(path, "/")
		if idPart == "" || strings.Contains(idPart, "/") {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		commentID, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil || commentID <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		user, ok := getAuthUser(w, r)
		if !ok {
			return
		}
		var ownerEmail string
		err = db.QueryRow("SELECT email FROM post_comments WHERE id = ?", commentID).Scan(&ownerEmail)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load comment", http.StatusInternalServerError)
			return
		}
		if ownerEmail != user.Email {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = db.Exec("DELETE FROM post_comment_likes WHERE comment_id = ? OR comment_id IN (SELECT id FROM post_comments WHERE parent_id = ?)", commentID, commentID)
		if _, err := db.Exec("DELETE FROM post_comments WHERE id = ? OR parent_id = ?", commentID, commentID); err != nil {
			http.Error(w, "failed to delete", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "deleted"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(path, "/like") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	idPart := strings.TrimSuffix(path, "/like")
	idPart = strings.TrimSuffix(idPart, "/")
	if idPart == "" || strings.Contains(idPart, "/") {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	commentID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || commentID <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var exists int
	err = db.QueryRow("SELECT 1 FROM post_comments WHERE id = ?", commentID).Scan(&exists)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load comment", http.StatusInternalServerError)
		return
	}
	liked := false
	err = db.QueryRow("SELECT 1 FROM post_comment_likes WHERE comment_id = ? AND email = ?", commentID, user.Email).Scan(&exists)
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO post_comment_likes (comment_id, email, created_at) VALUES (?, ?, ?)", commentID, user.Email, time.Now())
		if err != nil {
			http.Error(w, "failed to like", http.StatusInternalServerError)
			return
		}
		liked = true
	} else if err == nil {
		_, err = db.Exec("DELETE FROM post_comment_likes WHERE comment_id = ? AND email = ?", commentID, user.Email)
		if err != nil {
			http.Error(w, "failed to unlike", http.StatusInternalServerError)
			return
		}
		liked = false
	} else {
		http.Error(w, "failed to like", http.StatusInternalServerError)
		return
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM post_comment_likes WHERE comment_id = ?", commentID).Scan(&count); err != nil {
		http.Error(w, "failed to load likes", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"liked":     liked,
		"likeCount": count,
	})
}

func handleDeleteVideo(w http.ResponseWriter, r *http.Request, videoID string) {
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var ownerEmail, category, filename string
	err := db.QueryRow("SELECT email, category, filename FROM video_uploads WHERE video_id = ?", videoID).
		Scan(&ownerEmail, &category, &filename)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load video", http.StatusInternalServerError)
		return
	}
	if ownerEmail != user.Email {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	videoPath := filepath.Join("storage", "videos", category, filename)
	_ = os.Remove(videoPath)
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	thumbPath := filepath.Join("storage", "videos", category, base+".jpg")
	_ = os.Remove(thumbPath)
	_, _ = db.Exec("DELETE FROM video_comment_likes WHERE comment_id IN (SELECT id FROM video_comments WHERE video_id = ?)", videoID)
	_, _ = db.Exec("DELETE FROM video_comments WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM video_views WHERE video_id = ?", videoID)
	_, err = db.Exec("DELETE FROM video_uploads WHERE video_id = ?", videoID)
	if err != nil {
		http.Error(w, "failed to delete", http.StatusInternalServerError)
		return
	}
	_, _ = db.Exec("DELETE FROM video_library WHERE video_id = ?", videoID)
	_, _ = db.Exec("DELETE FROM manual_video_reviews WHERE video_id = ? AND status = 'pending'", videoID)
	_, _ = db.Exec("DELETE FROM manual_review_access_tokens WHERE video_id = ?", videoID)
	writeJSON(w, map[string]any{
		"status": "deleted",
	})
}

func handleRequestManualReview(w http.ResponseWriter, r *http.Request, videoID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}

	var ownerEmail string
	var title sql.NullString
	var reviewStatus string
	err := db.QueryRow(`SELECT vu.email, COALESCE(vl.title, ''), COALESCE(vl.review_status, '')
		FROM video_uploads vu
		LEFT JOIN video_library vl ON vl.video_id = vu.video_id
		WHERE vu.video_id = ?`, videoID).Scan(&ownerEmail, &title, &reviewStatus)
	if err == sql.ErrNoRows {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load video", http.StatusInternalServerError)
		return
	}
	if ownerEmail != user.Email {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if reviewStatus == "pending" || reviewStatus == "" {
		http.Error(w, "video is still pending review", http.StatusBadRequest)
		return
	}
	// 允许：AI 未通过（rejected*）或管理员下架（takedown）申请人工复审
	if reviewStatus != "approved" && reviewStatus != "takedown" && !strings.HasPrefix(reviewStatus, "rejected") {
		http.Error(w, "current status does not support manual review", http.StatusBadRequest)
		return
	}

	var pendingID int64
	err = db.QueryRow(`SELECT id FROM manual_video_reviews
		WHERE video_id = ? AND requester_email = ? AND status = 'pending'
		ORDER BY id DESC LIMIT 1`, videoID, user.Email).Scan(&pendingID)
	if err == nil {
		writeJSON(w, map[string]any{
			"status": "already_pending",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, "failed to check request status", http.StatusInternalServerError)
		return
	}

	videoTitle := ""
	if title.Valid {
		videoTitle = title.String
	}
	now := time.Now()
	_, err = db.Exec(`INSERT INTO manual_video_reviews (video_id, requester_email, title, status, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', ?, ?)`, videoID, user.Email, videoTitle, now, now)
	if err != nil {
		http.Error(w, "failed to submit request", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"status": "pending",
	})
}

func handleRequestPostManualReview(w http.ResponseWriter, r *http.Request, postID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var ownerEmail, title string
	var reviewStatus string
	err := db.QueryRow(`SELECT email, COALESCE(title, ''), COALESCE(review_status, '')
		FROM posts WHERE id = ?`, postID).Scan(&ownerEmail, &title, &reviewStatus)
	if err == sql.ErrNoRows {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load post", http.StatusInternalServerError)
		return
	}
	if ownerEmail != user.Email {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if reviewStatus == "pending" || reviewStatus == "" {
		http.Error(w, "帖子仍在审核中", http.StatusBadRequest)
		return
	}
	// 允许：AI 未通过（rejected*）或管理员下架（takedown）申请人工复审
	if reviewStatus != "approved" && reviewStatus != "takedown" && !strings.HasPrefix(reviewStatus, "rejected") {
		http.Error(w, "current status does not support manual review", http.StatusBadRequest)
		return
	}
	var pendingID int64
	err = db.QueryRow(`SELECT id FROM manual_post_reviews
		WHERE post_id = ? AND requester_email = ? AND status = 'pending'
		ORDER BY id DESC LIMIT 1`, postID, user.Email).Scan(&pendingID)
	if err == nil {
		writeJSON(w, map[string]any{
			"status": "already_pending",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, "failed to check request status", http.StatusInternalServerError)
		return
	}
	now := time.Now()
	_, err = db.Exec(`INSERT INTO manual_post_reviews (post_id, requester_email, title, status, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', ?, ?)`, postID, user.Email, title, now, now)
	if err != nil {
		http.Error(w, "failed to submit request", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status": "pending",
	})
}

func canAdminReviewAccessVideo(videoID, token string) bool {
	videoID = strings.TrimSpace(videoID)
	token = strings.TrimSpace(token)
	if videoID == "" || token == "" {
		return false
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM manual_review_access_tokens
		WHERE video_id = ? AND token = ? AND expires_at > ?
		ORDER BY id DESC LIMIT 1`, videoID, token, time.Now()).Scan(&exists)
	return err == nil
}

func canAdminReviewAccessPost(postID int64, token string) bool {
	token = strings.TrimSpace(token)
	if postID <= 0 || token == "" {
		return false
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM manual_post_review_access_tokens
		WHERE post_id = ? AND token = ? AND expires_at > ?
		ORDER BY id DESC LIMIT 1`, postID, token, time.Now()).Scan(&exists)
	return err == nil
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func handleHomepagePosters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := db.Query(`SELECT id, image_url, link_url, open_in_new_tab, enabled, sort_order, created_at, updated_at
		FROM homepage_posters
		WHERE enabled = 1
		ORDER BY sort_order DESC, id DESC
		LIMIT 6`)
	if err != nil {
		http.Error(w, "failed to load posters", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	result := make([]HomepagePoster, 0)
	for rows.Next() {
		var item HomepagePoster
		var openInt, enabledInt int
		if err := rows.Scan(&item.ID, &item.ImageURL, &item.LinkURL, &openInt, &enabledInt, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		item.OpenInNewTab = openInt == 1
		item.Enabled = enabledInt == 1
		result = append(result, item)
	}
	writeJSON(w, result)
}

func findVideo(id string) (Video, bool) {
	video, ok, _ := getVideoByID(id)
	return video, ok
}

func encodeTags(tags []string) string {
	data, err := json.Marshal(tags)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err == nil {
		return tags
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func decodeFormat(value string, duration float64, sizeBytes int64) FormatInfo {
	var format FormatInfo
	if strings.TrimSpace(value) != "" {
		_ = json.Unmarshal([]byte(value), &format)
	}
	if format.DurationSec == 0 && duration > 0 {
		format.DurationSec = duration
	}
	if format.SizeBytes == 0 && sizeBytes > 0 {
		format.SizeBytes = sizeBytes
	}
	return format
}

func ensureVideoCategory(category string) {
	if strings.TrimSpace(category) == "" {
		return
	}
	_, _ = db.Exec("INSERT IGNORE INTO video_categories (id, name, created_at) VALUES (?, ?, ?)", category, category, time.Now())
}

func upsertVideoLibraryFromFile(category, filename, description string, author videoAuthor, sizeBytes int64, modTime time.Time) error {
	fullPath := filepath.Join("storage", "videos", category, filename)
	title, parsedDescription, tags := parseTitleTags(filename)
	if strings.TrimSpace(description) == "" {
		description = parsedDescription
	}
	format, _ := probeFormat(fullPath)
	thumbURL := ensureThumbnail(category, filename, fullPath, modTime)
	playURL := buildMediaURL(category, filename)
	videoID := buildVideoID(category, filename)
	tagValue := encodeTags(tags)
	formatValue, _ := json.Marshal(format)
	ensureVideoCategory(category)
	_, err := db.Exec(`INSERT INTO video_library (video_id, category, title, description, tags, filename, file_path, thumb_url, play_url, duration_sec, size_bytes, format_json, created_at, updated_at, author_email, author_nickname)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE category = VALUES(category), title = VALUES(title), description = VALUES(description), tags = VALUES(tags), filename = VALUES(filename), file_path = VALUES(file_path),
		thumb_url = VALUES(thumb_url), play_url = VALUES(play_url), duration_sec = VALUES(duration_sec), size_bytes = VALUES(size_bytes), format_json = VALUES(format_json),
		updated_at = VALUES(updated_at), author_email = VALUES(author_email), author_nickname = VALUES(author_nickname)`,
		videoID, category, title, description, tagValue, filename, fullPath, thumbURL, playURL, format.DurationSec, sizeBytes, string(formatValue), modTime, time.Now(), author.Email, author.Nickname)
	return err
}

func queryVideosFromDB(category, query, authorEmail string) ([]Video, error) {
	baseQuery := `SELECT v.video_id, v.title, v.category, v.tags, v.thumb_url, v.play_url, v.duration_sec, v.size_bytes, v.format_json, v.created_at, v.description, v.author_email, v.author_nickname,
		COALESCE(lc.cnt, 0) AS like_count,
		COALESCE(cc.cnt, 0) AS comment_count,
		COALESCE(vc.cnt, 0) AS view_count,
		COALESCE(cl.cnt, 0) AS comment_likes,
		v.review_status,
		COALESCE(v.takedown_reason, '') AS takedown_reason
		FROM video_library v
		LEFT JOIN (
			SELECT video_id, COUNT(*) AS cnt
			FROM video_likes
			GROUP BY video_id
		) lc ON lc.video_id = v.video_id
		LEFT JOIN (
			SELECT video_id, COUNT(*) AS cnt
			FROM video_comments
			GROUP BY video_id
		) cc ON cc.video_id = v.video_id
		LEFT JOIN (
			SELECT video_id, COUNT(*) AS cnt
			FROM video_views
			GROUP BY video_id
		) vc ON vc.video_id = v.video_id
		LEFT JOIN (
			SELECT vc.video_id, COUNT(*) AS cnt
			FROM video_comment_likes vcl
			JOIN video_comments vc ON vcl.comment_id = vc.id
			GROUP BY vc.video_id
		) cl ON cl.video_id = v.video_id
		WHERE 1=1 AND (COALESCE(v.review_status, 'approved') = 'approved')`
	args := make([]any, 0)
	if authorEmail != "" {
		baseQuery += " AND v.author_email = ?"
		args = append(args, authorEmail)
	}
	if category != "" && category != "all" {
		baseQuery += " AND v.category = ?"
		args = append(args, category)
	}
	if strings.TrimSpace(query) != "" {
		q := "%" + strings.TrimSpace(query) + "%"
		baseQuery += " AND (v.title LIKE ? OR v.description LIKE ? OR v.tags LIKE ? OR v.category LIKE ? OR v.video_id LIKE ?)"
		args = append(args, q, q, q, q, q)
	}
	baseQuery += " ORDER BY v.created_at DESC"
	rows, err := db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Video, 0)
	for rows.Next() {
		var item Video
		var tagsValue sql.NullString
		var formatValue sql.NullString
		var duration sql.NullFloat64
		var sizeBytes sql.NullInt64
		var description sql.NullString
		var authorEmailValue sql.NullString
		var authorNicknameValue sql.NullString
		var likeCount, commentCount, viewCount, commentLikes int
		var reviewStatus, takedownReason string
		if err := rows.Scan(&item.ID, &item.Title, &item.Category, &tagsValue, &item.ThumbURL, &item.PlayURL, &duration, &sizeBytes, &formatValue, &item.CreatedAt, &description, &authorEmailValue, &authorNicknameValue, &likeCount, &commentCount, &viewCount, &commentLikes, &reviewStatus, &takedownReason); err != nil {
			continue
		}
		item.Tags = decodeTags(tagsValue.String)
		if description.Valid {
			item.Description = description.String
		}
		if authorEmailValue.Valid {
			item.AuthorEmail = authorEmailValue.String
		}
		if authorNicknameValue.Valid {
			item.AuthorNickname = authorNicknameValue.String
		}
		item.DurationSec = duration.Float64
		item.Format = decodeFormat(formatValue.String, duration.Float64, sizeBytes.Int64)
		item.Views = viewCount
		item.LikeCount = likeCount
		item.CommentCount = commentCount
		item.Score = float64(likeCount)*2.0 + float64(commentCount)*1.0 + float64(commentLikes)*0.5
		item.ReviewStatus = reviewStatus
		item.TakedownReason = takedownReason
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func getVideoByID(videoID string) (Video, bool, error) {
	baseQuery := `SELECT v.video_id, v.title, v.category, v.tags, v.thumb_url, v.play_url, v.duration_sec, v.size_bytes, v.format_json, v.created_at, v.description, v.author_email, v.author_nickname,
		COALESCE(lc.cnt, 0) AS like_count,
		COALESCE(cc.cnt, 0) AS comment_count,
		COALESCE(vc.cnt, 0) AS view_count,
		COALESCE(cl.cnt, 0) AS comment_likes,
		v.review_status
		FROM video_library v
		LEFT JOIN (
			SELECT video_id, COUNT(*) AS cnt
			FROM video_likes
			GROUP BY video_id
		) lc ON lc.video_id = v.video_id
		LEFT JOIN (
			SELECT video_id, COUNT(*) AS cnt
			FROM video_comments
			GROUP BY video_id
		) cc ON cc.video_id = v.video_id
		LEFT JOIN (
			SELECT video_id, COUNT(*) AS cnt
			FROM video_views
			GROUP BY video_id
		) vc ON vc.video_id = v.video_id
		LEFT JOIN (
			SELECT vc.video_id, COUNT(*) AS cnt
			FROM video_comment_likes vcl
			JOIN video_comments vc ON vcl.comment_id = vc.id
			GROUP BY vc.video_id
		) cl ON cl.video_id = v.video_id
		WHERE v.video_id = ?`
	var item Video
	var tagsValue sql.NullString
	var formatValue sql.NullString
	var duration sql.NullFloat64
	var sizeBytes sql.NullInt64
	var description sql.NullString
	var authorEmailValue sql.NullString
	var authorNicknameValue sql.NullString
	var likeCount, commentCount, viewCount, commentLikes int
	var reviewStatus string
	err := db.QueryRow(baseQuery, videoID).Scan(&item.ID, &item.Title, &item.Category, &tagsValue, &item.ThumbURL, &item.PlayURL, &duration, &sizeBytes, &formatValue, &item.CreatedAt, &description, &authorEmailValue, &authorNicknameValue, &likeCount, &commentCount, &viewCount, &commentLikes, &reviewStatus)
	if err == sql.ErrNoRows {
		return Video{}, false, nil
	}
	if err != nil {
		return Video{}, false, err
	}
	item.Tags = decodeTags(tagsValue.String)
	if description.Valid {
		item.Description = description.String
	}
	if authorEmailValue.Valid {
		item.AuthorEmail = authorEmailValue.String
	}
	if authorNicknameValue.Valid {
		item.AuthorNickname = authorNicknameValue.String
	}
	item.DurationSec = duration.Float64
	item.Format = decodeFormat(formatValue.String, duration.Float64, sizeBytes.Int64)
	item.Views = viewCount
	item.LikeCount = likeCount
	item.CommentCount = commentCount
	item.Score = float64(likeCount)*2.0 + float64(commentCount)*1.0 + float64(commentLikes)*0.5
	item.ReviewStatus = reviewStatus
	return item, true, nil
}

func generateThumbnail(inputPath, outputPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath, "-ss", "00:00:01.000", "-vframes", "1", outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func probeFormat(inputPath string) (FormatInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration,size,format_name,bit_rate,stream=codec_name,codec_type", "-of", "json", inputPath)
	output, err := cmd.Output()
	if err != nil {
		return FormatInfo{}, err
	}
	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return FormatInfo{}, err
	}
	info := FormatInfo{
		Container: strings.Split(parsed.Format.FormatName, ",")[0],
	}
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			info.VideoCodec = stream.CodecName
		case "audio":
			info.AudioCodec = stream.CodecName
		}
	}
	if parsed.Format.Duration != "" {
		if val, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
			info.DurationSec = val
		}
	}
	if parsed.Format.Size != "" {
		if val, err := strconv.ParseInt(parsed.Format.Size, 10, 64); err == nil {
			info.SizeBytes = val
		}
	}
	if parsed.Format.BitRate != "" {
		if val, err := strconv.ParseInt(parsed.Format.BitRate, 10, 64); err == nil {
			info.Bitrate = val
		}
	}
	return info, nil
}

func refreshFromStorage() {
	scanMtx.Lock()
	if scanInProgress {
		scanMtx.Unlock()
		return
	}
	if time.Since(lastScan) < scanInterval {
		scanMtx.Unlock()
		return
	}
	scanInProgress = true
	scanMtx.Unlock()

	defer func() {
		scanMtx.Lock()
		lastScan = time.Now()
		scanInProgress = false
		scanMtx.Unlock()
	}()

	root := filepath.Join("storage", "videos")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	seen := map[string]bool{}
	authorMap := loadVideoAuthorMap()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		categoryID := entry.Name()
		ensureVideoCategory(categoryID)
		categoryDir := filepath.Join(root, categoryID)
		files, err := os.ReadDir(categoryDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(file.Name()))
			if !supportedExts[ext] {
				continue
			}
			fullPath := filepath.Join(categoryDir, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}
			seen[fullPath] = true
			videoID := buildVideoID(categoryID, file.Name())
			author := authorMap[videoID]
			_ = upsertVideoLibraryFromFile(categoryID, file.Name(), "", author, info.Size(), info.ModTime())
		}
	}
	rows, err := db.Query("SELECT video_id, file_path FROM video_library")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var videoID, filePath string
		if err := rows.Scan(&videoID, &filePath); err != nil {
			continue
		}
		if _, ok := seen[filePath]; ok {
			continue
		}
		if _, err := os.Stat(filePath); err == nil {
			continue
		}
		_, _ = db.Exec("DELETE FROM video_library WHERE video_id = ?", videoID)
	}
}

func parseTitleTags(filename string) (string, string, []string) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Check for Description separator "---"
	var description string
	if parts := strings.Split(base, "---"); len(parts) > 1 {
		base = strings.TrimSpace(parts[0])
		description = strings.TrimSpace(parts[1])
	}

	// Heuristic to strip random ID prefix if it looks like one (4 alphanumeric chars + underscore)
	// Example: "ABCD_Title" -> "Title"
	if len(base) > 5 && base[4] == '_' {
		// Simple check if prefix is alphanumeric
		isID := true
		for i := 0; i < 4; i++ {
			c := base[i]
			if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				isID = false
				break
			}
		}
		if isID {
			base = strings.TrimSpace(base[5:])
		}
	}

	parts := strings.Split(base, "#")
	title := strings.TrimSpace(parts[0])
	tags := make([]string, 0)
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	if title == "" {
		title = base
	}
	return title, description, tags
}

func buildVideoID(categoryID, filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	return fmt.Sprintf("%s_%s", categoryID, base)
}

func stripHTMLToText(input string) string {
	if input == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(input))
	inTag := false
	for _, r := range input {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func countHTMLImageTags(input string) int {
	if input == "" {
		return 0
	}
	return strings.Count(strings.ToLower(input), "<img")
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

func ensureThumbnail(categoryID, filename, fullPath string, modTime time.Time) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	thumbName := base + ".jpg"
	thumbPath := filepath.Join(filepath.Dir(fullPath), thumbName)
	thumbURL := buildMediaURL(categoryID, thumbName)
	thumbInfo, err := os.Stat(thumbPath)
	if err == nil && !thumbInfo.ModTime().Before(modTime) {
		return thumbURL
	}
	_ = generateThumbnail(fullPath, thumbPath)
	return thumbURL
}

type smtpConfig struct {
	Host string `json:"host"`
	Port string `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

type mysqlConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Database string `json:"database"`
}

func loadSMTPConfig(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	var cfg smtpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	if strings.TrimSpace(cfg.Host) != "" {
		smtpHost = cfg.Host
	}
	if strings.TrimSpace(cfg.Port) != "" {
		smtpPort = cfg.Port
	}
	if strings.TrimSpace(cfg.User) != "" {
		smtpUser = cfg.User
	}
	if strings.TrimSpace(cfg.Pass) != "" {
		smtpPass = cfg.Pass
	}
}

func loadMySQLConfig(filePath string) error {
	mysqlCfg = mysqlConfig{
		Host:     "127.0.0.1",
		Port:     "3306",
		User:     "root",
		Pass:     "",
		Database: "boke",
	}
	data, err := os.ReadFile(filePath)
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

type jwtConfig struct {
	Secret string `json:"secret"`
}

func loadJWTSecret(filePath string) {
	data, err := os.ReadFile(filePath)
	if err == nil {
		var cfg jwtConfig
		if json.Unmarshal(data, &cfg) == nil && strings.TrimSpace(cfg.Secret) != "" {
			jwtSecret = []byte(cfg.Secret)
			return
		}
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err == nil {
		jwtSecret = secret
		return
	}
	jwtSecret = []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
}

func buildCodeKey(email, purpose string) string {
	return purpose + ":" + strings.ToLower(strings.TrimSpace(email))
}

func generateCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n)
}

func isSixDigits(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
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
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		email VARCHAR(255) PRIMARY KEY,
		nickname VARCHAR(100) NOT NULL DEFAULT '',
		password_hash VARCHAR(255) NOT NULL DEFAULT '',
		password_salt VARCHAR(255) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		balance DECIMAL(10,2) NOT NULL DEFAULT 0
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN balance DECIMAL(10,2) NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN avatar_url VARCHAR(255) NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN banner_url VARCHAR(255) NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN notice TEXT`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN motto VARCHAR(40) NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN message_read_at DATETIME NULL`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN video_publish_credits INT NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN post_publish_credits INT NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
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
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS deleted_messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		msg_key VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE KEY uk_email_msg_key (email, msg_key),
		INDEX idx_deleted_messages_email (email)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_uploads (
		video_id VARCHAR(255) PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		nickname VARCHAR(100) NOT NULL DEFAULT '',
		category VARCHAR(100) NOT NULL,
		filename VARCHAR(255) NOT NULL,
		size_bytes BIGINT NOT NULL,
		created_at DATETIME NOT NULL,
		INDEX idx_video_uploads_email (email)
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
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_comments (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		video_id VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		nickname VARCHAR(100) NOT NULL,
		content TEXT NOT NULL,
		parent_id BIGINT NULL,
		created_at DATETIME NOT NULL,
		INDEX idx_video_comments_video_id (video_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE video_comments ADD COLUMN parent_id BIGINT NULL`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_comment_likes (
		comment_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (comment_id, email),
		INDEX idx_video_comment_likes_comment_id (comment_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_favorites (
		video_id VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (video_id, email),
		INDEX idx_video_favorites_email (email)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_likes (
		video_id VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (video_id, email),
		INDEX idx_video_likes_video_id (video_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_like_rewards (
		video_id VARCHAR(255) NOT NULL,
		milestone INT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (video_id, milestone)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS video_views (
		video_id VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (video_id, email),
		INDEX idx_video_views_video_id (video_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS posts (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		nickname VARCHAR(100) NOT NULL DEFAULT '',
		title VARCHAR(255) NOT NULL,
		content TEXT NOT NULL,
		content_text TEXT NOT NULL,
		image_path VARCHAR(255) NOT NULL DEFAULT '',
		category VARCHAR(100) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		INDEX idx_posts_email (email)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE posts ADD COLUMN image_path VARCHAR(255) NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") && !strings.Contains(err.Error(), "Unknown column") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE posts ADD COLUMN category VARCHAR(100) NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") && !strings.Contains(err.Error(), "Unknown column") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE posts ADD COLUMN content_text TEXT NOT NULL`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") && !strings.Contains(err.Error(), "Unknown column") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE posts ADD COLUMN review_status VARCHAR(30) NOT NULL DEFAULT 'approved'`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") && !strings.Contains(err.Error(), "Unknown column") {
		panic(err)
	}
	// 一次性：将审核功能上线前已存在的帖子及卡在审核中的帖子统一设为已通过
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS _migrations (name VARCHAR(64) PRIMARY KEY)`)
	if r, e := db.Exec(`INSERT IGNORE INTO _migrations (name) VALUES ('approve_old_pending_posts')`); e == nil && r != nil {
		if n, _ := r.RowsAffected(); n == 1 {
			_, _ = db.Exec(`UPDATE posts SET review_status = 'approved' WHERE review_status = 'pending' OR review_status = '' OR review_status IS NULL`)
		}
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_categories (
		id VARCHAR(100) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		created_at DATETIME NOT NULL
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_comments (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		post_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		nickname VARCHAR(100) NOT NULL,
		content TEXT NOT NULL,
		parent_id BIGINT NULL,
		created_at DATETIME NOT NULL,
		INDEX idx_post_comments_post_id (post_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE post_comments ADD COLUMN parent_id BIGINT NULL`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_comment_likes (
		comment_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (comment_id, email),
		INDEX idx_post_comment_likes_comment_id (comment_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_likes (
		post_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (post_id, email),
		INDEX idx_post_likes_post_id (post_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_like_rewards (
		post_id BIGINT NOT NULL,
		milestone INT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (post_id, milestone)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_favorites (
		post_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (post_id, email),
		INDEX idx_post_favorites_email (email)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_views (
		post_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (post_id, email),
		INDEX idx_post_views_post_id (post_id)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS post_review_messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		post_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		title VARCHAR(255) NOT NULL DEFAULT '',
		result VARCHAR(30) NOT NULL,
		reason TEXT,
		created_at DATETIME NOT NULL,
		INDEX idx_prm_email (email),
		INDEX idx_prm_created (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE video_library ADD COLUMN review_status VARCHAR(20) NOT NULL DEFAULT 'approved'`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE video_library ADD COLUMN takedown_reason TEXT NULL`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE posts ADD COLUMN takedown_reason TEXT NULL`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") && !strings.Contains(err.Error(), "Unknown column") {
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
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS comment_review_messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		kind VARCHAR(10) NOT NULL,
		video_id VARCHAR(255) NULL,
		post_id BIGINT NULL,
		email VARCHAR(255) NOT NULL,
		target_title VARCHAR(500) NOT NULL DEFAULT '',
		reason TEXT,
		created_at DATETIME NOT NULL,
		INDEX idx_crm_email (email),
		INDEX idx_crm_created (created_at)
	)`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE video_comments ADD COLUMN review_status VARCHAR(20) NOT NULL DEFAULT 'approved'`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
		panic(err)
	}
	_, err = db.Exec(`ALTER TABLE post_comments ADD COLUMN review_status VARCHAR(20) NOT NULL DEFAULT 'approved'`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
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
}

func hashPassword(password string) (string, string, error) {
	if !isSixDigits(password) {
		return "", "", errors.New("password must be 6 digits")
	}
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt := base64.RawURLEncoding.EncodeToString(saltBytes)
	sum := sha256.Sum256([]byte(salt + password))
	return base64.RawURLEncoding.EncodeToString(sum[:]), salt, nil
}

func verifyPassword(password, salt, hash string) bool {
	sum := sha256.Sum256([]byte(salt + password))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == hash
}

func userExists(email string) (bool, error) {
	var exists int
	err := db.QueryRow("SELECT 1 FROM users WHERE email = ?", email).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func getUserByEmail(email string) (User, bool, error) {
	var user User
	var nickname, passwordHash, passwordSalt, avatarURL, bannerURL, notice, motto sql.NullString
	var balanceRaw sql.NullString
	var createdAt time.Time
	err := db.QueryRow("SELECT email, nickname, password_hash, password_salt, created_at, balance, avatar_url, banner_url, notice, motto FROM users WHERE email = ?", email).
		Scan(&user.Email, &nickname, &passwordHash, &passwordSalt, &createdAt, &balanceRaw, &avatarURL, &bannerURL, &notice, &motto)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		// Fallback for old schema if columns missing (though we added them)
		// Try reading without new columns if error
		if strings.Contains(err.Error(), "Unknown column") {
			err = db.QueryRow("SELECT email, nickname, password_hash, password_salt, created_at, balance FROM users WHERE email = ?", email).
				Scan(&user.Email, &nickname, &passwordHash, &passwordSalt, &createdAt, &balanceRaw)
			if err != nil {
				return User{}, false, err
			}
		} else {
			return User{}, false, err
		}
	} else {
		user.AvatarURL = avatarURL.String
		user.BannerURL = bannerURL.String
		user.Notice = notice.String
		user.Motto = motto.String
	}
	user.Nickname = nickname.String
	user.PasswordHash = passwordHash.String
	user.PasswordSalt = passwordSalt.String
	user.CreatedAt = createdAt
	if balanceRaw.Valid {
		if val, err := strconv.ParseFloat(balanceRaw.String, 64); err == nil {
			user.Balance = val
		}
	}
	return user, true, nil
}

// 封号：未赦免且 banned_until > now 的为有效封号
func getActiveBan(email string) (reason string, bannedUntil time.Time, ok bool) {
	var until time.Time
	err := db.QueryRow(`SELECT reason, banned_until FROM user_bans
		WHERE email = ? AND pardoned_at IS NULL AND banned_until > ?
		ORDER BY id DESC LIMIT 1`, email, time.Now()).Scan(&reason, &until)
	if err == sql.ErrNoRows {
		return "", time.Time{}, false
	}
	if err != nil {
		return "", time.Time{}, false
	}
	return reason, until, true
}

// 禁言：muted_until > now 的为有效禁言（取最新一条）
func getActiveMute(email string) (reason string, mutedUntil time.Time, ok bool) {
	var until time.Time
	err := db.QueryRow(`SELECT reason, muted_until FROM user_mutes
		WHERE email = ? AND muted_until > ?
		ORDER BY id DESC LIMIT 1`, email, time.Now()).Scan(&reason, &until)
	if err == sql.ErrNoRows {
		return "", time.Time{}, false
	}
	if err != nil {
		return "", time.Time{}, false
	}
	return reason, until, true
}

func writeBanResponse(w http.ResponseWriter, reason string, bannedUntil time.Time) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	remaining := int64(0)
	if bannedUntil.After(time.Now()) {
		remaining = int64(bannedUntil.Sub(time.Now()).Seconds())
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":             "banned",
		"reason":           reason,
		"bannedUntil":      bannedUntil.Format("2006-01-02 15:04:05"),
		"remainingSeconds": remaining,
	})
}

func writeMuteResponse(w http.ResponseWriter, reason string, mutedUntil time.Time) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":        "muted",
		"reason":      reason,
		"mutedUntil": mutedUntil.Format("2006-01-02 15:04:05"),
	})
}

func createUserWithPassword(email, password string) (User, error) {
	hash, salt, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	_, err = db.Exec("INSERT INTO users (email, nickname, password_hash, password_salt, created_at, balance) VALUES (?, ?, ?, ?, ?, ?)",
		email, "", hash, salt, now, 0)
	if err != nil {
		return User{}, err
	}
	return User{
		Email:        email,
		Nickname:     "",
		PasswordHash: hash,
		PasswordSalt: salt,
		CreatedAt:    now,
		Balance:      0,
	}, nil
}

func updateUserNickname(email, nickname string) (User, error) {
	_, err := db.Exec("UPDATE users SET nickname = ? WHERE email = ?", nickname, email)
	if err != nil {
		return User{}, err
	}
	_, _ = db.Exec("UPDATE video_uploads SET nickname = ? WHERE email = ?", nickname, email)
	_, _ = db.Exec("UPDATE video_comments SET nickname = ? WHERE email = ?", nickname, email)
	user, ok, err := getUserByEmail(email)
	if err != nil {
		return User{}, err
	}
	if !ok {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func updateUserPassword(email, password string) (User, error) {
	hash, salt, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	_, err = db.Exec("UPDATE users SET password_hash = ?, password_salt = ? WHERE email = ?", hash, salt, email)
	if err != nil {
		return User{}, err
	}
	user, ok, err := getUserByEmail(email)
	if err != nil {
		return User{}, err
	}
	if !ok {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func updateUserEmail(oldEmail, newEmail string) (User, error) {
	tx, err := db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var exists int
	err = tx.QueryRow("SELECT 1 FROM users WHERE email = ?", newEmail).Scan(&exists)
	if err != sql.ErrNoRows && err != nil {
		return User{}, err
	}
	if err == nil {
		return User{}, errors.New("email already in use")
	}
	_, err = tx.Exec("UPDATE users SET email = ? WHERE email = ?", newEmail, oldEmail)
	if err != nil {
		return User{}, err
	}
	_, err = tx.Exec("UPDATE video_uploads SET email = ? WHERE email = ?", newEmail, oldEmail)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	user, ok, err := getUserByEmail(newEmail)
	if err != nil {
		return User{}, err
	}
	if !ok {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

type videoAuthor struct {
	Nickname string
	Email    string
}

func loadVideoAuthorMap() map[string]videoAuthor {
	rows, err := db.Query("SELECT video_id, nickname, email FROM video_uploads")
	if err != nil {
		return map[string]videoAuthor{}
	}
	defer rows.Close()
	result := make(map[string]videoAuthor)
	for rows.Next() {
		var videoID string
		var nickname, email sql.NullString
		if err := rows.Scan(&videoID, &nickname, &email); err != nil {
			continue
		}
		result[videoID] = videoAuthor{
			Nickname: nickname.String,
			Email:    email.String,
		}
	}
	return result
}

func getUserUploadStats(email string) (int, int64, error) {
	var count int
	var total sql.NullInt64
	err := db.QueryRow("SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM video_uploads WHERE email = ?", email).
		Scan(&count, &total)
	if err != nil {
		return 0, 0, err
	}
	return count, total.Int64, nil
}

func getVideoPublishQuota(email string) (remaining int, used int, bonus int, err error) {
	if err = db.QueryRow("SELECT COUNT(*) FROM video_uploads WHERE email = ?", email).Scan(&used); err != nil {
		return 0, 0, 0, err
	}
	if err = db.QueryRow("SELECT COALESCE(video_publish_credits, 0) FROM users WHERE email = ?", email).Scan(&bonus); err != nil {
		return 0, 0, 0, err
	}
	remaining = baseVideoPublishLimit + bonus - used
	return remaining, used, bonus, nil
}

func getPostPublishQuota(email string) (remaining int, used int, bonus int, err error) {
	if err = db.QueryRow("SELECT COUNT(*) FROM posts WHERE email = ?", email).Scan(&used); err != nil {
		return 0, 0, 0, err
	}
	if err = db.QueryRow("SELECT COALESCE(post_publish_credits, 0) FROM users WHERE email = ?", email).Scan(&bonus); err != nil {
		return 0, 0, 0, err
	}
	remaining = basePostPublishLimit + bonus - used
	return remaining, used, bonus, nil
}

func grantVideoPublishBonusByLikes(videoID string, likeCount int) error {
	if likeCount < likesPerExtraPublish {
		return nil
	}
	milestones := likeCount / likesPerExtraPublish
	var ownerEmail string
	if err := db.QueryRow("SELECT email FROM video_uploads WHERE video_id = ?", videoID).Scan(&ownerEmail); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	for milestone := 1; milestone <= milestones; milestone++ {
		res, err := db.Exec(
			"INSERT IGNORE INTO video_like_rewards (video_id, milestone, created_at) VALUES (?, ?, ?)",
			videoID, milestone, time.Now(),
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			if _, err := db.Exec("UPDATE users SET video_publish_credits = COALESCE(video_publish_credits, 0) + 1 WHERE email = ?", ownerEmail); err != nil {
				return err
			}
		}
	}
	return nil
}

func grantPostPublishBonusByLikes(postID int64, likeCount int) error {
	if likeCount < likesPerExtraPublish {
		return nil
	}
	milestones := likeCount / likesPerExtraPublish
	var ownerEmail string
	if err := db.QueryRow("SELECT email FROM posts WHERE id = ?", postID).Scan(&ownerEmail); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	for milestone := 1; milestone <= milestones; milestone++ {
		res, err := db.Exec(
			"INSERT IGNORE INTO post_like_rewards (post_id, milestone, created_at) VALUES (?, ?, ?)",
			postID, milestone, time.Now(),
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			if _, err := db.Exec("UPDATE users SET post_publish_credits = COALESCE(post_publish_credits, 0) + 1 WHERE email = ?", ownerEmail); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertVideoUpload(user User, category, filename string, sizeBytes int64) error {
	videoID := buildVideoID(category, filename)
	_, err := db.Exec("INSERT INTO video_uploads (video_id, email, nickname, category, filename, size_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		videoID, user.Email, user.Nickname, category, filename, sizeBytes, time.Now())
	return err
}

func generateRandomID(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func sanitizeFilename(filename string) string {
	base := filepath.Base(filename)
	base = strings.TrimSpace(base)
	base = strings.ReplaceAll(base, " ", "_")
	base = strings.ReplaceAll(base, "..", "")
	if base == "." || base == "" {
		return ""
	}
	return base
}

func getBearerToken(r *http.Request) string {
	raw := r.Header.Get("Authorization")
	if strings.HasPrefix(raw, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	}
	return ""
}

func generateJWT(email string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("missing jwt secret")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(map[string]any{
		"sub": email,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, jwtSecret)
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func verifyJWT(token string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("missing jwt secret")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid token")
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, jwtSecret)
	_, _ = mac.Write([]byte(unsigned))
	expected := mac.Sum(nil)
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("invalid token")
	}
	if !hmac.Equal(sig, expected) {
		return "", errors.New("invalid token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("invalid token")
	}
	var payload struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if json.Unmarshal(payloadBytes, &payload) != nil {
		return "", errors.New("invalid token")
	}
	if payload.Sub == "" || time.Now().Unix() > payload.Exp {
		return "", errors.New("invalid token")
	}
	return payload.Sub, nil
}

func getAuthUser(w http.ResponseWriter, r *http.Request) (User, bool) {
	token := getBearerToken(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return User{}, false
	}
	email, err := verifyJWT(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return User{}, false
	}
	user, ok, err := getUserByEmail(email)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return User{}, false
	}
	if !ok {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return User{}, false
	}
	if reason, until, banned := getActiveBan(user.Email); banned {
		writeBanResponse(w, reason, until)
		return User{}, false
	}
	return user, true
}

func getAuthUserOptional(r *http.Request) (User, bool) {
	token := getBearerToken(r)
	if token == "" {
		return User{}, false
	}
	email, err := verifyJWT(token)
	if err != nil {
		return User{}, false
	}
	user, ok, err := getUserByEmail(email)
	if err != nil || !ok {
		return User{}, false
	}
	return user, true
}

func handleUserPunishment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUserOptional(r)
	if !ok {
		writeJSON(w, map[string]any{"banned": nil, "muted": nil})
		return
	}
	var banned, muted map[string]any
	if reason, until, active := getActiveBan(user.Email); active {
		remaining := int64(0)
		if until.After(time.Now()) {
			remaining = int64(until.Sub(time.Now()).Seconds())
		}
		banned = map[string]any{
			"reason":           reason,
			"bannedUntil":     until.Format("2006-01-02 15:04:05"),
			"remainingSeconds": remaining,
		}
	}
	if reason, until, active := getActiveMute(user.Email); active {
		muted = map[string]any{
			"reason":     reason,
			"mutedUntil": until.Format("2006-01-02 15:04:05"),
		}
	}
	writeJSON(w, map[string]any{"banned": banned, "muted": muted})
}

func init() {
	flag.StringVar(&smtpHost, "smtp-host", smtpHost, "smtp host")
	flag.StringVar(&smtpPort, "smtp-port", smtpPort, "smtp port")
	flag.StringVar(&smtpUser, "smtp-user", smtpUser, "smtp user")
	flag.StringVar(&smtpPass, "smtp-pass", smtpPass, "smtp pass")
	mime.AddExtensionType(".m3u8", "application/vnd.apple.mpegurl")
}

func handleSendCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Email == "" {
		http.Error(w, "请输入邮箱", http.StatusBadRequest)
		return
	}
	exists, err := userExists(req.Email)
	if err != nil {
		http.Error(w, "检查用户失败", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "该邮箱已注册", http.StatusBadRequest)
		return
	}
	if smtpUser == "" || smtpPass == "" {
		http.Error(w, "邮件服务未配置", http.StatusInternalServerError)
		return
	}

	code := generateCode()
	key := buildCodeKey(req.Email, "login")

	verificationMtx.Lock()
	verificationCodes[key] = VerificationCode{
		Email:     req.Email,
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Purpose:   "login",
	}
	verificationMtx.Unlock()

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: Your verification code\r\n"+
		"\r\n"+
		"Your code is %s. It expires in 5 minutes.\r\n", req.Email, code))

	err = smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{req.Email}, msg)
	if err != nil {
		http.Error(w, "发送邮件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"sent"}`))
}

func handleVerifyCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	verificationMtx.Lock()
	defer verificationMtx.Unlock()

	key := buildCodeKey(req.Email, "login")
	info, ok := verificationCodes[key]
	if !ok {
		http.Error(w, "验证码不存在", http.StatusBadRequest)
		return
	}
	if time.Now().After(info.ExpiresAt) {
		delete(verificationCodes, key)
		http.Error(w, "验证码已过期", http.StatusBadRequest)
		return
	}
	if info.Code != req.Code {
		http.Error(w, "验证码错误", http.StatusBadRequest)
		return
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		http.Error(w, "请输入密码", http.StatusBadRequest)
		return
	}
	if !isSixDigits(password) {
		http.Error(w, "密码必须为6位数字", http.StatusBadRequest)
		return
	}

	delete(verificationCodes, key)

	exists, err := userExists(req.Email)
	if err != nil {
		http.Error(w, "检查用户失败", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "该邮箱已注册", http.StatusBadRequest)
		return
	}

	user, err := createUserWithPassword(req.Email, password)
	if err != nil {
		http.Error(w, "创建用户失败", http.StatusInternalServerError)
		return
	}
	token, err := generateJWT(user.Email)
	if err != nil {
		http.Error(w, "创建令牌失败", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status":        "verified",
		"token":         token,
		"user":          user,
		"needsNickname": strings.TrimSpace(user.Nickname) == "",
		"needsPassword": user.PasswordHash == "",
	})
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{
		"email":       user.Email,
		"nickname":    user.Nickname,
		"hasPassword": user.PasswordHash != "",
		"balance":     user.Balance,
		"avatarUrl":   user.AvatarURL,
		"bannerUrl":   user.BannerURL,
		"notice":      user.Notice,
		"motto":       user.Motto,
		"videos":      getUserVideos(user.Email),
		"posts":       getUserPosts(user.Email),
	})
}

func handlePublicProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}
	user, ok, err := getUserByEmail(email)
	if err != nil {
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	includeUnapproved := false
	if reqUser, authed := getAuthUserOptional(r); authed && reqUser.Email == user.Email {
		includeUnapproved = true
	}
	videos := getUserVideos(user.Email)
	if !includeUnapproved {
		filtered := make([]Video, 0, len(videos))
		for _, v := range videos {
			if v.ReviewStatus == "" || v.ReviewStatus == "approved" {
				filtered = append(filtered, v)
			}
		}
		videos = filtered
	}
	posts := getUserPostsPublic(user.Email, includeUnapproved)
	writeJSON(w, map[string]any{
		"email":     user.Email,
		"nickname":  user.Nickname,
		"avatarUrl": user.AvatarURL,
		"bannerUrl": user.BannerURL,
		"notice":    user.Notice,
		"motto":     user.Motto,
		"videos":    videos,
		"posts":     posts,
	})
}

func getUserVideos(email string) []Video {
	refreshFromStorage()
	list, err := queryVideosFromDB("", "", email)
	if err != nil {
		return []Video{}
	}
	return list
}

func getUserPosts(email string) []Post {
	posts, err := queryPostsByEmail(email, true)
	if err != nil {
		return []Post{}
	}
	return posts
}

func getUserPostsPublic(email string, includeUnapproved bool) []Post {
	posts, err := queryPostsByEmail(email, includeUnapproved)
	if err != nil {
		return []Post{}
	}
	return posts
}

func queryPostsByEmail(email string, includeUnapproved bool) ([]Post, error) {
	q := `SELECT p.id, p.email, p.nickname, p.title, p.content, p.image_path, p.category, p.created_at, u.avatar_url,
		COALESCE(lc.cnt, 0) AS like_count,
		COALESCE(vc.cnt, 0) AS view_count,
		COALESCE(p.review_status, 'approved') AS review_status,
		COALESCE(p.takedown_reason, '') AS takedown_reason
		FROM posts p
		LEFT JOIN users u ON p.email = u.email
		LEFT JOIN (SELECT post_id, COUNT(*) AS cnt FROM post_likes GROUP BY post_id) lc ON lc.post_id = p.id
		LEFT JOIN (SELECT post_id, COUNT(*) AS cnt FROM post_views GROUP BY post_id) vc ON vc.post_id = p.id
		WHERE p.email = ?`
	args := []any{email}
	if !includeUnapproved {
		q += " AND (COALESCE(p.review_status, 'approved') = 'approved')"
	}
	q += " ORDER BY p.created_at DESC"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Post
	for rows.Next() {
		var p Post
		var avatar sql.NullString
		var takedownReason string
		if err := rows.Scan(&p.ID, &p.Email, &p.Nickname, &p.Title, &p.Content, &p.ImagePath, &p.Category, &p.CreatedAt, &avatar, &p.LikeCount, &p.Views, &p.ReviewStatus, &takedownReason); err != nil {
			continue
		}
		if avatar.Valid {
			p.AvatarURL = avatar.String
		}
		p.TakedownReason = takedownReason
		list = append(list, p)
	}
	return list, nil
}

func handleUpdateNickname(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		http.Error(w, "nickname required", http.StatusBadRequest)
		return
	}
	updated, err := updateUserNickname(user.Email, nickname)
	if err != nil {
		http.Error(w, "failed to update nickname", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"email":       updated.Email,
		"nickname":    updated.Nickname,
		"hasPassword": updated.PasswordHash != "",
	})
}

func handlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	password := strings.TrimSpace(req.Password)
	if email == "" || password == "" {
		http.Error(w, "请输入邮箱和密码", http.StatusBadRequest)
		return
	}
	if !isSixDigits(password) {
		http.Error(w, "密码必须为6位数字", http.StatusBadRequest)
		return
	}
	user, ok, err := getUserByEmail(email)
	if err != nil {
		http.Error(w, "加载用户失败", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "用户不存在", http.StatusBadRequest)
		return
	}
	if user.PasswordHash == "" || user.PasswordSalt == "" {
		http.Error(w, "未设置密码", http.StatusBadRequest)
		return
	}
	if !verifyPassword(password, user.PasswordSalt, user.PasswordHash) {
		http.Error(w, "密码错误", http.StatusBadRequest)
		return
	}
	if reason, until, banned := getActiveBan(user.Email); banned {
		writeBanResponse(w, reason, until)
		return
	}
	token, err := generateJWT(user.Email)
	if err != nil {
		http.Error(w, "创建令牌失败", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status": "ok",
		"token":  token,
		"user": map[string]any{
			"email":       user.Email,
			"nickname":    user.Nickname,
			"hasPassword": user.PasswordHash != "",
		},
	})
}

func handleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		http.Error(w, "password required", http.StatusBadRequest)
		return
	}
	if !isSixDigits(password) {
		http.Error(w, "password must be 6 digits", http.StatusBadRequest)
		return
	}
	updated, err := updateUserPassword(user.Email, password)
	if err != nil {
		http.Error(w, "failed to set password", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status": "updated",
		"user": map[string]any{
			"email":       updated.Email,
			"nickname":    updated.Nickname,
			"hasPassword": updated.PasswordHash != "",
		},
	})
}

func handleCreatorUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	if reason, until, muted := getActiveMute(user.Email); muted {
		writeMuteResponse(w, reason, until)
		return
	}
	remainingQuota, _, _, err := getVideoPublishQuota(user.Email)
	if err != nil {
		http.Error(w, "failed to read upload quota", http.StatusInternalServerError)
		return
	}
	if remainingQuota <= 0 {
		http.Error(w, "publish quota reached", http.StatusBadRequest)
		return
	}
	_, totalSize, err := getUserUploadStats(user.Email)
	if err != nil {
		http.Error(w, "failed to read upload stats", http.StatusInternalServerError)
		return
	}
	maxTotal := int64(500 * 1024 * 1024)
	remaining := maxTotal - totalSize
	if remaining <= 0 {
		http.Error(w, "upload limit reached", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, remaining+(10<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse upload", http.StatusBadRequest)
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" || category == "all" {
		http.Error(w, "category required", http.StatusBadRequest)
		return
	}
	if strings.Contains(category, "..") || strings.ContainsAny(category, `/\`) {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}
	categoryDir := filepath.Join("storage", "videos", category)
	info, err := os.Stat(categoryDir)
	if err != nil || !info.IsDir() {
		http.Error(w, "category not found", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	filename := sanitizeFilename(header.Filename)
	if filename == "" {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if !supportedExts[ext] {
		http.Error(w, "unsupported file type", http.StatusBadRequest)
		return
	}
	base := strings.TrimSuffix(filename, ext)
	description := strings.TrimSpace(r.FormValue("description"))

	// Generate random 4-char ID
	id := generateRandomID(4)

	// 文件名仅用 ID_基础名.ext，文案单独存入 SQL（video_library.description）
	targetName := fmt.Sprintf("%s_%s%s", id, base, ext)

	targetPath := filepath.Join(categoryDir, targetName)
	// Ensure uniqueness (though random ID makes collision unlikely)
	if _, err := os.Stat(targetPath); err == nil {
		targetName = fmt.Sprintf("%s_%d%s", strings.TrimSuffix(targetName, ext), time.Now().UnixNano(), ext)
		targetPath = filepath.Join(categoryDir, targetName)
	}
	out, err := os.Create(targetPath)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	limited := io.LimitReader(file, remaining+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		_ = os.Remove(targetPath)
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	if written <= 0 {
		_ = os.Remove(targetPath)
		http.Error(w, "empty file", http.StatusBadRequest)
		return
	}
	if written > remaining {
		_ = os.Remove(targetPath)
		http.Error(w, "upload limit reached", http.StatusBadRequest)
		return
	}
	if err := insertVideoUpload(user, category, targetName, written); err != nil {
		_ = os.Remove(targetPath)
		http.Error(w, "failed to record upload", http.StatusInternalServerError)
		return
	}
	info, statErr := os.Stat(targetPath)
	modTime := time.Now()
	if statErr == nil {
		modTime = info.ModTime()
	}
	_ = upsertVideoLibraryFromFile(category, targetName, description, videoAuthor{
		Nickname: user.Nickname,
		Email:    user.Email,
	}, written, modTime)

	videoID := buildVideoID(category, targetName)
	_, _ = db.Exec("UPDATE video_library SET review_status = 'pending' WHERE video_id = ?", videoID)

	title, _, _ := parseTitleTags(targetName)
	enqueueVideoReview(videoID, targetPath, user.Email, title, description)

	writeJSON(w, map[string]any{
		"status":  "ok",
		"videoId": videoID,
		"review":  "pending",
	})
}

type appConfig struct {
	PublishQuota *struct {
		VideoBase      int `json:"video_base"`
		PostBase       int `json:"post_base"`
		LikesPerBonus  int `json:"likes_per_bonus"`
	} `json:"publish_quota"`
}

func loadAppConfig() {
	data, err := os.ReadFile("app.config.json")
	if err != nil {
		return
	}
	var cfg appConfig
	if json.Unmarshal(data, &cfg) != nil || cfg.PublishQuota == nil {
		return
	}
	if cfg.PublishQuota.VideoBase > 0 {
		baseVideoPublishLimit = cfg.PublishQuota.VideoBase
	}
	if cfg.PublishQuota.PostBase > 0 {
		basePostPublishLimit = cfg.PublishQuota.PostBase
	}
	if cfg.PublishQuota.LikesPerBonus > 0 {
		likesPerExtraPublish = cfg.PublishQuota.LikesPerBonus
	}
}

func refreshRankings() {
	rankingMux.Lock()
	defer rankingMux.Unlock()
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthStart := firstOfMonth.Format("2006-01-02 15:04:05")
	rows, err := db.Query(`SELECT v.video_id, v.title FROM video_library v
		LEFT JOIN (SELECT video_id, COUNT(*) AS cnt FROM video_likes GROUP BY video_id) lc ON lc.video_id = v.video_id
		LEFT JOIN (SELECT video_id, COUNT(*) AS cnt FROM video_comments GROUP BY video_id) cc ON cc.video_id = v.video_id
		LEFT JOIN (SELECT video_id, COUNT(*) AS cnt FROM video_views GROUP BY video_id) vc ON vc.video_id = v.video_id
		LEFT JOIN (SELECT vc.video_id, COUNT(*) AS cnt FROM video_comment_likes vcl JOIN video_comments vc ON vcl.comment_id = vc.id GROUP BY vc.video_id) cl ON cl.video_id = v.video_id
		WHERE v.review_status = 'approved' AND v.created_at >= ?
		ORDER BY (COALESCE(lc.cnt,0)*2 + COALESCE(cc.cnt,0)*1 + COALESCE(cl.cnt,0)*0.5) DESC, v.created_at DESC LIMIT 10`, monthStart)
	if err != nil {
		videoRankingCache = nil
	} else {
		var list []Video
		for rows.Next() {
			var item Video
			if err := rows.Scan(&item.ID, &item.Title); err != nil {
				continue
			}
			list = append(list, item)
		}
		rows.Close()
		videoRankingCache = list
	}
	rows2, err := db.Query(`SELECT p.id, p.title FROM posts p
		LEFT JOIN (SELECT post_id, COUNT(*) AS cnt FROM post_likes GROUP BY post_id) lc ON lc.post_id = p.id
		LEFT JOIN (SELECT post_id, COUNT(*) AS cnt FROM post_views GROUP BY post_id) vc ON vc.post_id = p.id
		WHERE (p.review_status = 'approved' OR p.review_status = '' OR p.review_status IS NULL) AND p.created_at >= ?
		ORDER BY (COALESCE(lc.cnt,0)*2 + COALESCE(vc.cnt,0)*0.5) DESC, p.created_at DESC LIMIT 10`, monthStart)
	if err != nil {
		postRankingCache = nil
	} else {
		var list []Post
		for rows2.Next() {
			var item Post
			if err := rows2.Scan(&item.ID, &item.Title); err != nil {
				continue
			}
			list = append(list, item)
		}
		rows2.Close()
		postRankingCache = list
	}
}

func runRankingsRefreshEvery12h() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		refreshRankings()
	}
}

func handleRankingsVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rankingMux.RLock()
	list := videoRankingCache
	rankingMux.RUnlock()
	if list == nil {
		list = []Video{}
	}
	writeJSON(w, list)
}

func handleRankingsPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rankingMux.RLock()
	list := postRankingCache
	rankingMux.RUnlock()
	if list == nil {
		list = []Post{}
	}
	writeJSON(w, list)
}

func handleCreatorPublishQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	videoRemaining, videoUsed, videoBonus, err := getVideoPublishQuota(user.Email)
	if err != nil {
		http.Error(w, "failed to load quota", http.StatusInternalServerError)
		return
	}
	postRemaining, postUsed, postBonus, err := getPostPublishQuota(user.Email)
	if err != nil {
		http.Error(w, "failed to load quota", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"videoRemaining": videoRemaining,
		"videoUsed":      videoUsed,
		"videoBonus":     videoBonus,
		"videoBase":      baseVideoPublishLimit,
		"postRemaining":  postRemaining,
		"postUsed":       postUsed,
		"postBonus":      postBonus,
		"postBase":       basePostPublishLimit,
		"likesPerBonus":  likesPerExtraPublish,
	})
}

func handleSendChangeEmailCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newEmail := strings.TrimSpace(req.Email)
	if newEmail == "" {
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}
	if newEmail == user.Email {
		http.Error(w, "email unchanged", http.StatusBadRequest)
		return
	}
	exists, err := userExists(newEmail)
	if err != nil {
		http.Error(w, "failed to check user", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "email already in use", http.StatusBadRequest)
		return
	}
	if smtpUser == "" || smtpPass == "" {
		http.Error(w, "smtp not configured", http.StatusInternalServerError)
		return
	}

	code := generateCode()
	key := buildCodeKey(newEmail, "change")

	verificationMtx.Lock()
	verificationCodes[key] = VerificationCode{
		Email:      newEmail,
		Code:       code,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
		Purpose:    "change",
		OwnerEmail: user.Email,
	}
	verificationMtx.Unlock()

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: Your verification code\r\n"+
		"\r\n"+
		"Your code is %s. It expires in 5 minutes.\r\n", newEmail, code))

	err = smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{newEmail}, msg)
	if err != nil {
		http.Error(w, "failed to send email: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"status": "sent"})
}

func handleVerifyChangeEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newEmail := strings.TrimSpace(req.Email)
	if newEmail == "" || strings.TrimSpace(req.Code) == "" {
		http.Error(w, "email and code required", http.StatusBadRequest)
		return
	}

	key := buildCodeKey(newEmail, "change")
	verificationMtx.Lock()
	info, ok := verificationCodes[key]
	if !ok {
		verificationMtx.Unlock()
		http.Error(w, "code not found", http.StatusBadRequest)
		return
	}
	if time.Now().After(info.ExpiresAt) {
		delete(verificationCodes, key)
		verificationMtx.Unlock()
		http.Error(w, "code expired", http.StatusBadRequest)
		return
	}
	if info.Code != req.Code || info.OwnerEmail != user.Email {
		verificationMtx.Unlock()
		http.Error(w, "invalid code", http.StatusBadRequest)
		return
	}
	delete(verificationCodes, key)
	verificationMtx.Unlock()

	updated, err := updateUserEmail(user.Email, newEmail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token, err := generateJWT(updated.Email)
	if err != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status": "updated",
		"token":  token,
		"user": map[string]any{
			"email":       updated.Email,
			"nickname":    updated.Nickname,
			"hasPassword": updated.PasswordHash != "",
		},
	})
}

func updateUserAvatar(email, avatarURL string) error {
	_, err := db.Exec("UPDATE users SET avatar_url = ? WHERE email = ?", avatarURL, email)
	return err
}

func updateUserBanner(email, bannerURL string) error {
	_, err := db.Exec("UPDATE users SET banner_url = ? WHERE email = ?", bannerURL, email)
	return err
}

func updateUserNotice(email, notice string) error {
	_, err := db.Exec("UPDATE users SET notice = ? WHERE email = ?", notice, email)
	return err
}

func updateUserMotto(email, motto string) error {
	_, err := db.Exec("UPDATE users SET motto = ? WHERE email = ?", motto, email)
	return err
}

func handleUpdateAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	// Limit upload size for avatar (e.g. 5MB)
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		http.Error(w, "failed to parse upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" {
		http.Error(w, "only images allowed", http.StatusBadRequest)
		return
	}

	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), generateRandomID(6), ext)
	path := filepath.Join("storage", "avatars", filename)
	out, err := os.Create(path)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	avatarURL := "/media/avatars/" + filename
	if err := updateUserAvatar(user.Email, avatarURL); err != nil {
		http.Error(w, "failed to update db", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"status": "ok", "avatarUrl": avatarURL})
}

func handleUpdateBanner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	// Limit upload size for banner (e.g. 10MB)
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" {
		http.Error(w, "only images allowed", http.StatusBadRequest)
		return
	}

	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), generateRandomID(6), ext)
	path := filepath.Join("storage", "banners", filename)
	out, err := os.Create(path)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	bannerURL := "/media/banners/" + filename
	if err := updateUserBanner(user.Email, bannerURL); err != nil {
		http.Error(w, "failed to update db", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"status": "ok", "bannerUrl": bannerURL})
}

func handleUpdateNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Notice string `json:"notice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len([]rune(req.Notice)) > 200 {
		http.Error(w, "notice too long", http.StatusBadRequest)
		return
	}

	if err := updateUserNotice(user.Email, req.Notice); err != nil {
		http.Error(w, "failed to update db", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"status": "ok", "notice": req.Notice})
}

func handleUpdateMotto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Motto string `json:"motto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len([]rune(req.Motto)) > 10 {
		http.Error(w, "motto too long", http.StatusBadRequest)
		return
	}
	if err := updateUserMotto(user.Email, req.Motto); err != nil {
		http.Error(w, "failed to update db", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "motto": req.Motto})
}

func handlePostImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
		http.Error(w, "only jpg/jpeg/png/webp/gif allowed", http.StatusBadRequest)
		return
	}
	now := time.Now()
	year, month, day := now.Date()
	dirPath := filepath.Join("uploads", fmt.Sprintf("%d", year), fmt.Sprintf("%02d", month), fmt.Sprintf("%02d", day))
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	uniqueName := fmt.Sprintf("%d_%s%s", now.UnixNano(), generateRandomID(6), ext)
	fullPath := filepath.Join(dirPath, uniqueName)
	out, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	written, err := io.Copy(out, file)
	if err != nil || written <= 0 {
		_ = os.Remove(fullPath)
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	imagePath := "/" + filepath.ToSlash(fullPath)
	writeJSON(w, map[string]any{
		"status": "ok",
		"url":    imagePath,
	})
}

func handlePosts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		author := strings.TrimSpace(r.URL.Query().Get("author"))
		baseQuery := `SELECT p.id, p.email, p.nickname, p.title, p.content, p.image_path, p.category, p.created_at, u.avatar_url,
			COALESCE(lc.cnt, 0) AS like_count,
			COALESCE(vc.cnt, 0) AS view_count
			FROM posts p
			LEFT JOIN users u ON p.email = u.email
			LEFT JOIN (
				SELECT post_id, COUNT(*) AS cnt
				FROM post_likes
				GROUP BY post_id
			) lc ON lc.post_id = p.id
			LEFT JOIN (
				SELECT post_id, COUNT(*) AS cnt
				FROM post_views
				GROUP BY post_id
			) vc ON vc.post_id = p.id
			WHERE (COALESCE(p.review_status, 'approved') = 'approved')`
		args := make([]any, 0)
		if author != "" {
			baseQuery += " AND p.email = ?"
			args = append(args, author)
		}
		if category != "" && category != "all" {
			baseQuery += " AND p.category = ?"
			args = append(args, category)
		}
		if query != "" {
			baseQuery += " AND (p.title LIKE ? OR p.content_text LIKE ? OR p.content LIKE ? OR p.category LIKE ? OR p.nickname LIKE ?)"
			likeValue := "%" + query + "%"
			args = append(args, likeValue, likeValue, likeValue, likeValue, likeValue)
		}
		baseQuery += " ORDER BY p.created_at DESC"
		rows, err := db.Query(baseQuery, args...)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to load posts: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		posts := make([]Post, 0)
		for rows.Next() {
			var p Post
			var avatar sql.NullString
			if err := rows.Scan(&p.ID, &p.Email, &p.Nickname, &p.Title, &p.Content, &p.ImagePath, &p.Category, &p.CreatedAt, &avatar, &p.LikeCount, &p.Views); err != nil {
				continue
			}
			if avatar.Valid {
				p.AvatarURL = avatar.String
			}
			posts = append(posts, p)
		}
		writeJSON(w, posts)
		return
	}

	if r.Method == http.MethodPost {
		user, ok := getAuthUser(w, r)
		if !ok {
			return
		}
		if reason, until, muted := getActiveMute(user.Email); muted {
			writeMuteResponse(w, reason, until)
			return
		}
		remainingQuota, _, _, err := getPostPublishQuota(user.Email)
		if err != nil {
			http.Error(w, "failed to read publish quota", http.StatusInternalServerError)
			return
		}
		if remainingQuota <= 0 {
			http.Error(w, "publish quota reached", http.StatusBadRequest)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}

		title := strings.TrimSpace(r.FormValue("title"))
		content := strings.TrimSpace(r.FormValue("content"))
		contentText := stripHTMLToText(content)
		category := strings.TrimSpace(r.FormValue("category"))
		if title == "" || contentText == "" || category == "" || category == "all" {
			http.Error(w, "title, content and category required", http.StatusBadRequest)
			return
		}
		if len([]rune(contentText)) > 20000 {
			http.Error(w, "post content too long (max 20000 chars)", http.StatusBadRequest)
			return
		}
		imageCount := countHTMLImageTags(content)
		var exists int
		if err := db.QueryRow("SELECT 1 FROM post_categories WHERE id = ?", category).Scan(&exists); err == sql.ErrNoRows {
			http.Error(w, "category not found", http.StatusBadRequest)
			return
		} else if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		imagePath := strings.TrimSpace(r.FormValue("imagePath"))
		if imagePath != "" {
			if !strings.HasPrefix(imagePath, "/uploads/") {
				http.Error(w, "invalid image path", http.StatusBadRequest)
				return
			}
			imageCount += 1
		} else {
			file, header, err := r.FormFile("image")
			if err == nil {
				defer file.Close()
				ext := strings.ToLower(filepath.Ext(header.Filename))
				if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
					http.Error(w, "only jpg/png allowed", http.StatusBadRequest)
					return
				}

				now := time.Now()
				year, month, day := now.Date()
				dirPath := filepath.Join("uploads", fmt.Sprintf("%d", year), fmt.Sprintf("%02d", month), fmt.Sprintf("%02d", day))
				if err := os.MkdirAll(dirPath, 0755); err != nil {
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}

				uniqueName := fmt.Sprintf("%d_%s%s", now.UnixNano(), generateRandomID(6), ext)
				fullPath := filepath.Join(dirPath, uniqueName)

				out, err := os.Create(fullPath)
				if err != nil {
					http.Error(w, "failed to save image", http.StatusInternalServerError)
					return
				}
				io.Copy(out, file)
				out.Close()

				imagePath = "/" + filepath.ToSlash(fullPath)
				imageCount += 1
			}
		}
		if imageCount > 25 {
			http.Error(w, "post images exceed limit (max 25)", http.StatusBadRequest)
			return
		}

		res, err := db.Exec("INSERT INTO posts (email, nickname, title, content, content_text, image_path, category, review_status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)",
			user.Email, user.Nickname, title, content, contentText, imagePath, category, time.Now())
		if err != nil {
			http.Error(w, "failed to save post", http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		go enqueuePostReview(id, contentText, content, imagePath, user.Email, user.Nickname, title)
		writeJSON(w, map[string]any{"status": "ok", "id": id})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func handlePostDetail(w http.ResponseWriter, r *http.Request) {
	pathID := strings.TrimPrefix(r.URL.Path, "/api/posts/")
	if strings.HasSuffix(pathID, "/manual-review") {
		idPart := strings.TrimSuffix(pathID, "/manual-review")
		idPart = strings.TrimSuffix(idPart, "/")
		if idPart == "" || strings.Contains(idPart, "/") {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		handleRequestPostManualReview(w, r, id)
		return
	}
	if strings.HasSuffix(pathID, "/comments") {
		idPart := strings.TrimSuffix(pathID, "/comments")
		idPart = strings.TrimSuffix(idPart, "/")
		if idPart == "" || strings.Contains(idPart, "/") {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		handlePostComments(w, r, id)
		return
	}
	pathID = strings.TrimSuffix(pathID, "/")
	if pathID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		var p Post
		var avatar sql.NullString
		var motto sql.NullString
		var reviewStatus string
		err := db.QueryRow(`SELECT p.id, p.email, p.nickname, p.title, p.content, p.image_path, p.category, p.created_at, u.avatar_url, u.motto,
			COALESCE(lc.cnt, 0) AS like_count,
			COALESCE(vc.cnt, 0) AS view_count,
			COALESCE(p.review_status, 'approved') AS review_status
			FROM posts p
			LEFT JOIN users u ON p.email = u.email
			LEFT JOIN (
				SELECT post_id, COUNT(*) AS cnt
				FROM post_likes
				GROUP BY post_id
			) lc ON lc.post_id = p.id
			LEFT JOIN (
				SELECT post_id, COUNT(*) AS cnt
				FROM post_views
				GROUP BY post_id
			) vc ON vc.post_id = p.id
			WHERE p.id = ?`, id).Scan(&p.ID, &p.Email, &p.Nickname, &p.Title, &p.Content, &p.ImagePath, &p.Category, &p.CreatedAt, &avatar, &motto, &p.LikeCount, &p.Views, &reviewStatus)

		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		p.ReviewStatus = reviewStatus
		if reviewStatus == "takedown" {
			var reason string
			_ = db.QueryRow("SELECT COALESCE(takedown_reason, '') FROM posts WHERE id = ?", id).Scan(&reason)
			reqUser, authed := getAuthUserOptional(r)
			allowed := authed && reqUser.Email == p.Email
			if !allowed {
				adminToken := strings.TrimSpace(r.URL.Query().Get("adminReviewToken"))
				if adminToken == "" {
					adminToken = strings.TrimSpace(r.Header.Get("X-Admin-Review-Token"))
				}
				if canAdminReviewAccessPost(id, adminToken) {
					allowed = true
				}
			}
			if !allowed {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "takedown", "reason": reason})
				return
			}
			p.TakedownReason = reason
		}
		// 未通过/被封的帖子仅作者或管理员（带复审 token）可见
		if reviewStatus != "" && reviewStatus != "approved" && reviewStatus != "takedown" {
			reqUser, authed := getAuthUserOptional(r)
			allowed := false
			if authed && reqUser.Email == p.Email {
				allowed = true
			}
			if !allowed {
				adminToken := strings.TrimSpace(r.URL.Query().Get("adminReviewToken"))
				if adminToken == "" {
					adminToken = strings.TrimSpace(r.Header.Get("X-Admin-Review-Token"))
				}
				if canAdminReviewAccessPost(id, adminToken) {
					allowed = true
				}
			}
			if !allowed {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}
		if avatar.Valid {
			p.AvatarURL = avatar.String
		}
		authorMotto := ""
		if motto.Valid {
			authorMotto = motto.String
		}
		isLiked := false
		isFavorited := false
		if user, ok := getAuthUserOptional(r); ok {
			var exists int
			err = db.QueryRow("SELECT 1 FROM post_likes WHERE post_id = ? AND email = ?", id, user.Email).Scan(&exists)
			if err == nil {
				isLiked = true
			}
			err = db.QueryRow("SELECT 1 FROM post_favorites WHERE post_id = ? AND email = ?", id, user.Email).Scan(&exists)
			if err == nil {
				isFavorited = true
			}
		}
		type postDetail struct {
			Post
			IsLiked     bool   `json:"isLiked"`
			IsFavorited bool   `json:"isFavorited"`
			AuthorMotto string `json:"authorMotto"`
		}
		writeJSON(w, postDetail{Post: p, IsLiked: isLiked, IsFavorited: isFavorited, AuthorMotto: authorMotto})
		return
	}

	if r.Method == http.MethodDelete {
		user, ok := getAuthUser(w, r)
		if !ok {
			return
		}
		var ownerEmail string
		var imagePath sql.NullString
		err := db.QueryRow("SELECT email, image_path FROM posts WHERE id = ?", id).Scan(&ownerEmail, &imagePath)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if ownerEmail != user.Email {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		_, _ = db.Exec("DELETE FROM post_comment_likes WHERE comment_id IN (SELECT id FROM post_comments WHERE post_id = ?)", id)
		_, _ = db.Exec("DELETE FROM post_comments WHERE post_id = ?", id)
		_, _ = db.Exec("DELETE FROM post_likes WHERE post_id = ?", id)
		_, _ = db.Exec("DELETE FROM post_views WHERE post_id = ?", id)
		_, err = db.Exec("DELETE FROM posts WHERE id = ?", id)
		if err != nil {
			http.Error(w, "failed to delete", http.StatusInternalServerError)
			return
		}
		if imagePath.Valid && strings.HasPrefix(imagePath.String, "/uploads/") {
			_ = os.Remove(strings.TrimPrefix(imagePath.String, "/"))
		}
		writeJSON(w, map[string]any{"status": "deleted"})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// ============ Python Env ============

var reviewPython = filepath.Join("venv311_new", "Scripts", "python.exe")

func ensurePythonDeps() {
	// 使用绝对路径，确保依赖检查与子进程使用同一虚拟环境
	if abs, err := filepath.Abs(reviewPython); err == nil {
		reviewPython = abs
	}
	pip := filepath.Join("venv311_new", "Scripts", "pip.exe")
	if abs, err := filepath.Abs(pip); err == nil {
		pip = abs
	}
	if _, err := os.Stat(reviewPython); err != nil {
		log.Printf("[python] 警告: 虚拟环境不存在: %s, 跳过依赖检查", reviewPython)
		return
	}

	// 优先用 modelscope 官方 requirements 一次性装齐（hub + datasets 等，含 datasets 3.x）
	wd, _ := os.Getwd()
	reqFile := filepath.Join(wd, "requirements_review.txt")
	if _, err := os.Stat(reqFile); err == nil {
		log.Printf("[python] 使用 requirements_review.txt 安装 modelscope 及全部依赖...")
		installReq := exec.Command(pip, "install", "-r", reqFile)
		installReq.Dir = wd
		installReq.Stdout = os.Stdout
		installReq.Stderr = os.Stderr
		if runErr := installReq.Run(); runErr != nil {
			log.Printf("[python] requirements_review.txt 安装失败: %v", runErr)
		} else {
			log.Printf("[python] requirements_review.txt 安装完成")
		}
	}

	// 视频审核 + 基础环境
	required := []string{"numpy", "opencv-python", "tensorflow", "tf_keras"}
	log.Printf("[python] 检查 Python 3.11 虚拟环境依赖... (python=%s)", reviewPython)

	out, err := exec.Command(pip, "list", "--format=freeze").Output()
	if err != nil {
		log.Printf("[python] pip list 失败: %v, 尝试安装基础依赖", err)
		installPythonPkgs(pip, required)
		return
	}

	installed := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.Split(strings.TrimSpace(line), "==")[0]
		installed[strings.ToLower(name)] = true
	}

	var missing []string
	for _, pkg := range required {
		lookup := strings.ToLower(pkg)
		if lookup == "opencv-python" {
			if !installed["opencv-python"] && !installed["opencv-python-headless"] {
				missing = append(missing, pkg)
			}
		} else if lookup == "tf_keras" {
			if !installed["tf-keras"] && !installed["tf_keras"] {
				missing = append(missing, pkg)
			}
		} else if !installed[lookup] {
			missing = append(missing, pkg)
		}
	}

	if len(missing) > 0 {
		log.Printf("[python] 缺失基础依赖: %v, 开始安装...", missing)
		installPythonPkgs(pip, missing)
	} else {
		log.Printf("[python] 所有依赖已就绪")
	}
}

func installPythonPkgs(pip string, pkgs []string) {
	args := append([]string{"install"}, pkgs...)
	cmd := exec.Command(pip, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("[python] 依赖安装失败: %v", err)
	} else {
		log.Printf("[python] 依赖安装完成: %v", pkgs)
	}
}

// ============ Video Review Queue ============

func enqueueVideoReview(videoID, filePath, authorEmail, title, description string) {
	waiting := atomic.AddInt64(&reviewWaitCount, 1)
	active := atomic.LoadInt64(&reviewActiveCount)
	log.Printf("[review] 视频入队审核 videoID=%s title=%q author=%s (当前活跃=%d, 排队=%d)",
		videoID, title, authorEmail, active, waiting)
	go func() {
		if active >= 2 {
			log.Printf("[review] 审核队列已满(2/2), videoID=%s 等待中...", videoID)
		}
		reviewSem <- struct{}{}
		atomic.AddInt64(&reviewWaitCount, -1)
		newActive := atomic.AddInt64(&reviewActiveCount, 1)
		log.Printf("[review] 开始审核 videoID=%s (活跃=%d)", videoID, newActive)
		defer func() {
			a := atomic.AddInt64(&reviewActiveCount, -1)
			w := atomic.LoadInt64(&reviewWaitCount)
			log.Printf("[review] 审核完成 videoID=%s (活跃=%d, 排队=%d)", videoID, a, w)
			<-reviewSem
		}()
		runVideoReview(videoID, filePath, authorEmail, title, description)
	}()
}

func runVideoReview(videoID, filePath, authorEmail, title, description string) {
	log.Printf("[review] ======================================")
	log.Printf("[review] 审核开始: videoID=%s", videoID)
	log.Printf("[review]   标题: %s", title)
	log.Printf("[review]   文件: %s", filePath)
	log.Printf("[review]   作者: %s", authorEmail)
	log.Printf("[review] ======================================")
	startTime := time.Now()
	wd, _ := os.Getwd()

	// 1. 视频文案审核（标题+描述，与帖子文案同一套 review_text.py）
	textToReview := strings.TrimSpace(title + "\n" + description)
	if textToReview != "" {
		tmpFile, err := os.CreateTemp("", "video_review_*.txt")
		if err != nil {
			log.Printf("[review] 视频文案临时文件创建失败 videoID=%s: %v", videoID, err)
		} else {
			_, _ = tmpFile.WriteString(textToReview)
			tmpFile.Close()
			textPath := tmpFile.Name()
			defer os.Remove(textPath)

			ctxText, cancelText := context.WithTimeout(context.Background(), 2*time.Minute)
			cmdText := exec.CommandContext(ctxText, reviewPython, "review_text.py", textPath)
			cmdText.Dir = wd
			var stdoutText bytes.Buffer
			cmdText.Stdout = &stdoutText
			cmdText.Stderr = os.Stderr
			err = cmdText.Run()
			cancelText()
			if err != nil {
				log.Printf("[review] 视频文案审核脚本执行失败 videoID=%s: %v", videoID, err)
			} else {
				var textResult struct {
					Passed       bool   `json:"passed"`
					RejectReason string `json:"reject_reason"`
				}
				if json.Unmarshal(stdoutText.Bytes(), &textResult) == nil && !textResult.Passed {
					log.Printf("[review] 视频文案审核不通过(辱骂/歧视) videoID=%s", videoID)
					setReviewResult(videoID, "rejected_abuse", authorEmail, title)
					return
				}
			}
		}
	}

	// 2. 视频画面审核
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, reviewPython, "review_video.py", filePath)
	if d, err := os.Getwd(); err == nil {
		cmd.Dir = d
	}
	cmd.Stderr = os.Stderr

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	log.Printf("[review] 启动Python审核脚本...")
	err := cmd.Run()
	output := stdout.Bytes()

	if err != nil {
		log.Printf("[review] Python脚本执行出错 videoID=%s: %v", videoID, err)
		if len(output) > 0 {
			log.Printf("[review] stdout输出: %s", string(output))
		}
		log.Printf("[review] 审核出错，默认通过 videoID=%s (耗时 %s)", videoID, time.Since(startTime).Round(time.Millisecond))
		setReviewResult(videoID, "approved", authorEmail, title)
		return
	}

	var result struct {
		Approved     bool   `json:"approved"`
		RejectReason string `json:"reject_reason"`
		Error        string `json:"error"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("[review] JSON解析失败 videoID=%s: %v", videoID, err)
		log.Printf("[review] 原始输出: %s", string(output))
		log.Printf("[review] 解析失败，默认通过 videoID=%s (耗时 %s)", videoID, time.Since(startTime).Round(time.Millisecond))
		setReviewResult(videoID, "approved", authorEmail, title)
		return
	}

	if result.Error != "" {
		log.Printf("[review] 审核脚本报告错误 videoID=%s: %s", videoID, result.Error)
		log.Printf("[review] 脚本出错，默认通过 videoID=%s (耗时 %s)", videoID, time.Since(startTime).Round(time.Millisecond))
		setReviewResult(videoID, "approved", authorEmail, title)
		return
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)

	if result.Approved {
		log.Printf("[review] 审核通过 videoID=%s (耗时 %s)", videoID, elapsed)
		setReviewResult(videoID, "approved", authorEmail, title)
	} else {
		status := "rejected"
		switch result.RejectReason {
		case "violence":
			status = "rejected_violence"
			log.Printf("[review] 审核未通过(暴力内容) videoID=%s (耗时 %s)", videoID, elapsed)
		case "nsfw":
			status = "rejected_nsfw"
			log.Printf("[review] 审核未通过(裸露内容) videoID=%s (耗时 %s)", videoID, elapsed)
		default:
			log.Printf("[review] 审核未通过(原因=%s) videoID=%s (耗时 %s)", result.RejectReason, videoID, elapsed)
		}
		setReviewResult(videoID, status, authorEmail, title)
	}
}

func setReviewResult(videoID, status, authorEmail, title string) {
	_, err := db.Exec("UPDATE video_library SET review_status = ? WHERE video_id = ?", status, videoID)
	if err != nil {
		log.Printf("[review] 数据库更新失败 videoID=%s: %v", videoID, err)
	} else {
		log.Printf("[review] 数据库已更新 videoID=%s review_status=%s", videoID, status)
	}

	var msgReason string
	switch status {
	case "approved":
		msgReason = "您的视频已通过审核，已在首页展示"
	case "rejected_abuse":
		msgReason = "您的视频文案可能含有歧视辱骂内容，未通过审核"
	case "rejected_violence":
		msgReason = "您的视频含有暴力内容，未通过审核"
	case "rejected_nsfw":
		msgReason = "您的视频含有裸露内容，未通过审核"
	default:
		msgReason = "您的视频未通过审核"
	}

	_, err = db.Exec(`INSERT INTO video_review_messages (video_id, email, title, result, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, videoID, authorEmail, title, status, msgReason, time.Now())
	if err != nil {
		log.Printf("[review] 通知消息写入失败 videoID=%s: %v", videoID, err)
	} else {
		log.Printf("[review] 已发送通知给 %s: %s", authorEmail, msgReason)
	}
}

// ============ Post Review ============

func enqueuePostReview(postID int64, contentText, contentHTML, imagePath, authorEmail, authorNickname, title string) {
	log.Printf("[review] 帖子入队审核 postID=%d title=%q author=%s", postID, title, authorEmail)
	runPostReview(postID, contentText, contentHTML, imagePath, authorEmail, title)
}

func runPostReview(postID int64, contentText, contentHTML, imagePath, authorEmail, title string) {
	startTime := time.Now()
	log.Printf("[review] 开始审核帖子 postID=%d", postID)
	wd, _ := os.Getwd()

	// 1. 文本审核 (nlp_structbert_abuse-detect_chinese-tiny)：标题 + 正文
	textToReview := strings.TrimSpace(title)
	if contentText != "" {
		if textToReview != "" {
			textToReview += "\n\n"
		}
		textToReview += strings.TrimSpace(contentText)
	}
	if textToReview != "" {
		tmpFile, err := os.CreateTemp("", "post_review_*.txt")
		if err != nil {
			log.Printf("[review] 帖子文本临时文件创建失败 postID=%d: %v", postID, err)
		} else {
			_, _ = tmpFile.WriteString(textToReview)
			tmpFile.Close()
			textPath := tmpFile.Name()
			defer os.Remove(textPath)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			cmd := exec.CommandContext(ctx, reviewPython, "review_text.py", textPath)
			cmd.Dir = wd
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			cancel()
			if err != nil {
				log.Printf("[review] 帖子文本审核脚本执行失败 postID=%d: %v", postID, err)
			} else {
				var textResult struct {
					Passed       bool   `json:"passed"`
					RejectReason string `json:"reject_reason"`
				}
				if json.Unmarshal(stdout.Bytes(), &textResult) == nil && !textResult.Passed {
					log.Printf("[review] 帖子文本审核不通过(辱骂/歧视) postID=%d", postID)
					setReviewResultPost(postID, "rejected_abuse", authorEmail, title, "你的帖子可能含有歧视辱骂内容，不通过")
					return
				}
			}
		}
	}

	// 2. 图片审核：缩略图（封面）+ 正文中的图片
	imagePaths := extractPostImagePaths(contentHTML, imagePath, wd)
	if len(imagePaths) > 0 {
		log.Printf("[review] 帖子图片审核 postID=%d 共 %d 张（含缩略图）", postID, len(imagePaths))
	}
	for _, p := range imagePaths {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, reviewPython, "review_image.py", p)
		cmd.Dir = wd
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		cancel()
		if err != nil {
			log.Printf("[review] 帖子图片审核脚本执行失败 postID=%d path=%s: %v", postID, p, err)
			continue
		}
		var imgResult struct {
			Approved     bool   `json:"approved"`
			RejectReason string `json:"reject_reason"`
		}
		if json.Unmarshal(stdout.Bytes(), &imgResult) != nil || imgResult.Approved {
			continue
		}
		reason := "你的帖子图片含有裸露、恐暴内容，不通过"
		log.Printf("[review] 帖子图片审核不通过 postID=%d reason=%s", postID, imgResult.RejectReason)
		switch imgResult.RejectReason {
		case "nsfw":
			setReviewResultPost(postID, "rejected_nsfw", authorEmail, title, reason)
		case "violence":
			setReviewResultPost(postID, "rejected_violence", authorEmail, title, reason)
		default:
			setReviewResultPost(postID, "rejected_nsfw", authorEmail, title, reason)
		}
		return
	}

	log.Printf("[review] 帖子审核通过 postID=%d (耗时 %s)", postID, time.Since(startTime).Round(time.Millisecond))
	setReviewResultPost(postID, "approved", authorEmail, title, "您的帖子已通过审核，已正常展示")
}

func extractPostImagePaths(contentHTML, imagePath, wd string) []string {
	seen := make(map[string]bool)
	var paths []string
	addPath := func(abs string) {
		if abs != "" && !seen[abs] {
			seen[abs] = true
			paths = append(paths, abs)
		}
	}
	// 缩略图（封面）：必须审核，支持 /uploads/ 路径及完整 URL
	if imagePath != "" {
		pathPart := extractUploadsPath(imagePath)
		if pathPart != "" {
			abs := filepath.Join(wd, strings.TrimPrefix(pathPart, "/"))
			if _, err := os.Stat(abs); err == nil {
				addPath(abs)
			}
		}
	}
	// 从 HTML 中提取 img src：支持 /uploads/ 相对路径及 http(s)://.../uploads/... 完整 URL
	for {
		idx := strings.Index(strings.ToLower(contentHTML), "src=\"")
		if idx < 0 {
			break
		}
		contentHTML = contentHTML[idx+5:]
		end := strings.Index(contentHTML, "\"")
		if end < 0 {
			break
		}
		src := contentHTML[:end]
		contentHTML = contentHTML[end+1:]
		pathPart := extractUploadsPath(src)
		if pathPart != "" {
			abs := filepath.Join(wd, strings.TrimPrefix(pathPart, "/"))
			if _, err := os.Stat(abs); err == nil {
				addPath(abs)
			}
		}
	}
	return paths
}

// extractUploadsPath 从路径或 URL 中提取 /uploads/... 部分，用于图片审核
func extractUploadsPath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "/uploads/") {
		if q := strings.Index(s, "?"); q >= 0 {
			s = s[:q]
		}
		return s
	}
	if idx := strings.Index(s, "/uploads/"); idx >= 0 {
		pathPart := s[idx:]
		if q := strings.Index(pathPart, "?"); q >= 0 {
			pathPart = pathPart[:q]
		}
		return pathPart
	}
	return ""
}

// ============ Comment Review (AI 不通过直接删除，无复审；仅文本审核) ============

func runCommentReview(kind string, commentID int64, videoID string, postID int64, content, authorEmail, targetTitle string) {
	wd, _ := os.Getwd()
	if content == "" {
		// 无文字则直接通过
		if kind == "video" {
			_, _ = db.Exec("UPDATE video_comments SET review_status = 'approved' WHERE id = ?", commentID)
		} else {
			_, _ = db.Exec("UPDATE post_comments SET review_status = 'approved' WHERE id = ?", commentID)
		}
		return
	}
	tmpFile, err := os.CreateTemp("", "comment_review_*.txt")
	if err != nil {
		log.Printf("[review] 评论文本临时文件创建失败 commentID=%d: %v", commentID, err)
		if kind == "video" {
			_, _ = db.Exec("UPDATE video_comments SET review_status = 'approved' WHERE id = ?", commentID)
		} else {
			_, _ = db.Exec("UPDATE post_comments SET review_status = 'approved' WHERE id = ?", commentID)
		}
		return
	}
	_, _ = tmpFile.WriteString(content)
	tmpFile.Close()
	textPath := tmpFile.Name()
	defer os.Remove(textPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	cmd := exec.CommandContext(ctx, reviewPython, "review_text.py", textPath)
	cmd.Dir = wd
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	cancel()
	if err != nil {
		log.Printf("[review] 评论文本审核脚本执行失败 commentID=%d: %v", commentID, err)
		if kind == "video" {
			_, _ = db.Exec("UPDATE video_comments SET review_status = 'approved' WHERE id = ?", commentID)
		} else {
			_, _ = db.Exec("UPDATE post_comments SET review_status = 'approved' WHERE id = ?", commentID)
		}
		return
	}
	var textResult struct {
		Passed       bool   `json:"passed"`
		RejectReason string `json:"reject_reason"`
	}
	if json.Unmarshal(stdout.Bytes(), &textResult) != nil || textResult.Passed {
		// 通过：更新为 approved
		if kind == "video" {
			_, _ = db.Exec("UPDATE video_comments SET review_status = 'approved' WHERE id = ?", commentID)
		} else {
			_, _ = db.Exec("UPDATE post_comments SET review_status = 'approved' WHERE id = ?", commentID)
		}
		log.Printf("[review] 评论审核通过 %s commentID=%d", kind, commentID)
		return
	}
	// AI 不通过：直接删除评论，无复审；并通知用户
	log.Printf("[review] 评论文本审核不通过(辱骂/歧视) %s commentID=%d", kind, commentID)
	if kind == "video" {
		_, _ = db.Exec("DELETE FROM video_comment_likes WHERE comment_id = ?", commentID)
		_, _ = db.Exec("DELETE FROM video_comments WHERE id = ?", commentID)
	} else {
		_, _ = db.Exec("DELETE FROM post_comment_likes WHERE comment_id = ?", commentID)
		_, _ = db.Exec("DELETE FROM post_comments WHERE id = ?", commentID)
	}
	reason := "你在\"" + targetTitle + "\"发表的言论可能涉及歧视辱骂已被删除"
	var vid interface{} = nil
	var pid interface{} = nil
	if kind == "video" {
		vid = videoID
	} else {
		pid = postID
	}
	_, err = db.Exec(`INSERT INTO comment_review_messages (kind, video_id, post_id, email, target_title, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, kind, vid, pid, authorEmail, targetTitle, reason, time.Now())
	if err != nil {
		log.Printf("[review] 评论审核通知写入失败 commentID=%d: %v", commentID, err)
	} else {
		log.Printf("[review] 已发送评论审核删除通知给 %s: %s", authorEmail, reason)
	}
}

func setReviewResultPost(postID int64, status, authorEmail, title, msgReason string) {
	_, err := db.Exec("UPDATE posts SET review_status = ? WHERE id = ?", status, postID)
	if err != nil {
		log.Printf("[review] 帖子审核状态更新失败 postID=%d: %v", postID, err)
	} else {
		log.Printf("[review] 帖子审核状态已更新 postID=%d status=%s", postID, status)
	}
	_, err = db.Exec(`INSERT INTO post_review_messages (post_id, email, title, result, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, postID, authorEmail, title, status, msgReason, time.Now())
	if err != nil {
		log.Printf("[review] 帖子审核通知写入失败 postID=%d: %v", postID, err)
	} else {
		log.Printf("[review] 已发送帖子审核通知给 %s: %s", authorEmail, msgReason)
	}
}

func handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	active := atomic.LoadInt64(&reviewActiveCount)
	waiting := atomic.LoadInt64(&reviewWaitCount)
	writeJSON(w, map[string]any{
		"active":        active,
		"waiting":       waiting,
		"maxConcurrent": 2,
	})
}
