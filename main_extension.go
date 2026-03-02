package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"sort"
	"strings"
	"time"
)

func handleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		VideoID string `json:"videoId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.VideoID == "" {
		http.Error(w, "videoId required", http.StatusBadRequest)
		return
	}

	var exists int
	err := db.QueryRow("SELECT 1 FROM video_favorites WHERE video_id = ? AND email = ?", req.VideoID, user.Email).Scan(&exists)
	isFavorite := false
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO video_favorites (video_id, email, created_at) VALUES (?, ?, ?)", req.VideoID, user.Email, time.Now())
		if err != nil {
			http.Error(w, "failed to favorite", http.StatusInternalServerError)
			return
		}
		isFavorite = true
	} else if err == nil {
		_, err = db.Exec("DELETE FROM video_favorites WHERE video_id = ? AND email = ?", req.VideoID, user.Email)
		if err != nil {
			http.Error(w, "failed to unfavorite", http.StatusInternalServerError)
			return
		}
		isFavorite = false
	} else {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"status": "ok", "isFavorite": isFavorite})
}

func handleUserFavorites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}

	refreshFromStorage()
	rows, err := db.Query(`SELECT v.video_id, v.title, v.category, v.tags, v.thumb_url, v.play_url, v.duration_sec, v.size_bytes, v.format_json, v.created_at, v.description, v.author_email, v.author_nickname,
		f.created_at AS favorite_at,
		COALESCE(lc.cnt, 0) AS like_count,
		COALESCE(cc.cnt, 0) AS comment_count,
		COALESCE(vc.cnt, 0) AS view_count,
		COALESCE(cl.cnt, 0) AS comment_likes
		FROM video_favorites f
		JOIN video_library v ON v.video_id = f.video_id AND v.review_status = 'approved'
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
		WHERE f.email = ?
		ORDER BY f.created_at DESC`, user.Email)
	if err != nil {
		http.Error(w, "failed to load favorites", http.StatusInternalServerError)
		return
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
		if err := rows.Scan(&item.ID, &item.Title, &item.Category, &tagsValue, &item.ThumbURL, &item.PlayURL, &duration, &sizeBytes, &formatValue, &item.CreatedAt, &description, &authorEmailValue, &authorNicknameValue, &item.FavoriteAt, &likeCount, &commentCount, &viewCount, &commentLikes); err != nil {
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
		result = append(result, item)
	}
	writeJSON(w, result)
}

func handleTogglePostFavorite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		PostID int64 `json:"postId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.PostID <= 0 {
		http.Error(w, "postId required", http.StatusBadRequest)
		return
	}
	var postExists int
	if err := db.QueryRow("SELECT 1 FROM posts WHERE id = ?", req.PostID).Scan(&postExists); err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	var exists int
	err := db.QueryRow("SELECT 1 FROM post_favorites WHERE post_id = ? AND email = ?", req.PostID, user.Email).Scan(&exists)
	isFavorite := false
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO post_favorites (post_id, email, created_at) VALUES (?, ?, ?)", req.PostID, user.Email, time.Now())
		if err != nil {
			http.Error(w, "failed to favorite", http.StatusInternalServerError)
			return
		}
		isFavorite = true
	} else if err == nil {
		_, err = db.Exec("DELETE FROM post_favorites WHERE post_id = ? AND email = ?", req.PostID, user.Email)
		if err != nil {
			http.Error(w, "failed to unfavorite", http.StatusInternalServerError)
			return
		}
		isFavorite = false
	} else {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"status": "ok", "isFavorite": isFavorite})
}

func handleUserPostFavorites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`SELECT p.id, p.email, p.nickname, p.title, p.content, p.image_path, p.category, p.created_at, u.avatar_url, f.created_at,
		COALESCE(lc.cnt, 0) AS like_count,
		COALESCE(vc.cnt, 0) AS view_count
		FROM post_favorites f
		JOIN posts p ON p.id = f.post_id
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
		WHERE f.email = ?
		ORDER BY f.created_at DESC`, user.Email)
	if err != nil {
		http.Error(w, "failed to load favorites", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := make([]Post, 0)
	for rows.Next() {
		var p Post
		var avatar sql.NullString
		if err := rows.Scan(&p.ID, &p.Email, &p.Nickname, &p.Title, &p.Content, &p.ImagePath, &p.Category, &p.CreatedAt, &avatar, &p.FavoriteAt, &p.LikeCount, &p.Views); err != nil {
			continue
		}
		if avatar.Valid {
			p.AvatarURL = avatar.String
		}
		posts = append(posts, p)
	}
	writeJSON(w, posts)
}

func handleSendLoginCode(w http.ResponseWriter, r *http.Request) {
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
	email := strings.TrimSpace(req.Email)
	if email == "" {
		http.Error(w, "请输入邮箱", http.StatusBadRequest)
		return
	}
	exists, err := userExists(email)
	if err != nil {
		http.Error(w, "检查用户失败", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "用户不存在", http.StatusBadRequest)
		return
	}
	if smtpUser == "" || smtpPass == "" {
		http.Error(w, "邮件服务未配置", http.StatusInternalServerError)
		return
	}

	code := generateCode()
	key := buildCodeKey(email, "login-code")

	verificationMtx.Lock()
	verificationCodes[key] = VerificationCode{
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Purpose:   "login-code",
	}
	verificationMtx.Unlock()

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: Your verification code\r\n"+
		"\r\n"+
		"Your code is %s. It expires in 5 minutes.\r\n", email, code))

	err = smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{email}, msg)
	if err != nil {
		http.Error(w, "发送邮件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"sent"}`))
}

func handleVerifyLoginCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	email := strings.TrimSpace(req.Email)
	code := strings.TrimSpace(req.Code)
	if email == "" || code == "" {
		http.Error(w, "请输入邮箱和验证码", http.StatusBadRequest)
		return
	}

	verificationMtx.Lock()
	defer verificationMtx.Unlock()

	key := buildCodeKey(email, "login-code")
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
	if info.Code != code {
		http.Error(w, "验证码错误", http.StatusBadRequest)
		return
	}

	delete(verificationCodes, key)

	user, ok, err := getUserByEmail(email)
	if err != nil {
		http.Error(w, "加载用户失败", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "用户不存在", http.StatusBadRequest)
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

func handleToggleVideoLike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		VideoID string `json:"videoId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.VideoID == "" {
		http.Error(w, "videoId required", http.StatusBadRequest)
		return
	}

	var exists int
	err := db.QueryRow("SELECT 1 FROM video_likes WHERE video_id = ? AND email = ?", req.VideoID, user.Email).Scan(&exists)
	isLiked := false
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO video_likes (video_id, email, created_at) VALUES (?, ?, ?)", req.VideoID, user.Email, time.Now())
		if err != nil {
			http.Error(w, "failed to like", http.StatusInternalServerError)
			return
		}
		isLiked = true
	} else if err == nil {
		_, err = db.Exec("DELETE FROM video_likes WHERE video_id = ? AND email = ?", req.VideoID, user.Email)
		if err != nil {
			http.Error(w, "failed to unlike", http.StatusInternalServerError)
			return
		}
		isLiked = false
	} else {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM video_likes WHERE video_id = ?", req.VideoID).Scan(&count); err != nil {
		http.Error(w, "failed to count likes", http.StatusInternalServerError)
		return
	}
	if isLiked {
		if err := grantVideoPublishBonusByLikes(req.VideoID, count); err != nil {
			fmt.Printf("grant video publish bonus failed for %s: %v\n", req.VideoID, err)
		}
	}

	writeJSON(w, map[string]any{"status": "ok", "isLiked": isLiked, "likeCount": count})
}

func handleVideoView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		VideoID string `json:"videoId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.VideoID == "" {
		http.Error(w, "videoId required", http.StatusBadRequest)
		return
	}
	refreshFromStorage()
	if _, ok := findVideo(req.VideoID); !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, err := db.Exec("INSERT IGNORE INTO video_views (video_id, email, created_at) VALUES (?, ?, ?)", req.VideoID, user.Email, time.Now())
	if err != nil {
		http.Error(w, "failed to view", http.StatusInternalServerError)
		return
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM video_views WHERE video_id = ?", req.VideoID).Scan(&count); err != nil {
		http.Error(w, "failed to count views", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "viewCount": count})
}

func handleTogglePostLike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		PostID int64 `json:"postId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.PostID <= 0 {
		http.Error(w, "postId required", http.StatusBadRequest)
		return
	}
	var postExists int
	if err := db.QueryRow("SELECT 1 FROM posts WHERE id = ?", req.PostID).Scan(&postExists); err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	var exists int
	err := db.QueryRow("SELECT 1 FROM post_likes WHERE post_id = ? AND email = ?", req.PostID, user.Email).Scan(&exists)
	isLiked := false
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO post_likes (post_id, email, created_at) VALUES (?, ?, ?)", req.PostID, user.Email, time.Now())
		if err != nil {
			http.Error(w, "failed to like", http.StatusInternalServerError)
			return
		}
		isLiked = true
	} else if err == nil {
		_, err = db.Exec("DELETE FROM post_likes WHERE post_id = ? AND email = ?", req.PostID, user.Email)
		if err != nil {
			http.Error(w, "failed to unlike", http.StatusInternalServerError)
			return
		}
		isLiked = false
	} else {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ?", req.PostID).Scan(&count); err != nil {
		http.Error(w, "failed to count likes", http.StatusInternalServerError)
		return
	}
	if isLiked {
		if err := grantPostPublishBonusByLikes(req.PostID, count); err != nil {
			fmt.Printf("grant post publish bonus failed for %d: %v\n", req.PostID, err)
		}
	}

	writeJSON(w, map[string]any{"status": "ok", "isLiked": isLiked, "likeCount": count})
}

// ============ Messages / Notifications ============

type messageItem struct {
	Type         string    `json:"type"`
	MsgKey       string    `json:"msgKey"`
	FromEmail    string    `json:"fromEmail"`
	FromNickname string    `json:"fromNickname"`
	FromAvatar   string    `json:"fromAvatar"`
	Content      string    `json:"content"`
	MyContent    string    `json:"myContent"`
	TargetTitle  string    `json:"targetTitle"`
	VideoID      string    `json:"videoId,omitempty"`
	PostID       int64     `json:"postId,omitempty"`
	CommentID    int64     `json:"commentId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func getDeletedMsgKeys(email string) map[string]bool {
	deleted := make(map[string]bool)
	rows, err := db.Query("SELECT msg_key FROM deleted_messages WHERE email = ?", email)
	if err != nil {
		return deleted
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err == nil {
			deleted[key] = true
		}
	}
	return deleted
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	email := user.Email
	var messages []messageItem
	deleted := getDeletedMsgKeys(email)

	// 1. Video comment replies: someone replied to my video comment
	rows1, err := db.Query(`SELECT c2.email, c2.nickname, COALESCE(u.avatar_url, ''), c2.content, c.content, COALESCE(vl.title, ''), c.video_id, c2.id, c2.created_at
		FROM video_comments c2
		JOIN video_comments c ON c2.parent_id = c.id
		LEFT JOIN users u ON u.email = c2.email
		LEFT JOIN video_library vl ON vl.video_id = c.video_id
		WHERE c.email = ? AND c2.email != ?
		ORDER BY c2.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows1.Close()
		for rows1.Next() {
			var m messageItem
			m.Type = "video_reply"
			if err := rows1.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.Content, &m.MyContent, &m.TargetTitle, &m.VideoID, &m.CommentID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("video_reply_%d", m.CommentID)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 1b. Video direct comments: someone commented on my video (top-level, not a reply)
	rows1b, err := db.Query(`SELECT vc.email, vc.nickname, COALESCE(u.avatar_url, ''), vc.content, COALESCE(vl.title, ''), vc.video_id, vc.id, vc.created_at
		FROM video_comments vc
		JOIN video_uploads vu ON vc.video_id = vu.video_id
		LEFT JOIN users u ON u.email = vc.email
		LEFT JOIN video_library vl ON vl.video_id = vc.video_id
		WHERE vu.email = ? AND vc.email != ? AND vc.parent_id IS NULL
		ORDER BY vc.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows1b.Close()
		for rows1b.Next() {
			var m messageItem
			m.Type = "video_comment"
			if err := rows1b.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.Content, &m.TargetTitle, &m.VideoID, &m.CommentID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("video_comment_%d", m.CommentID)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 2. Video comment likes: someone liked my video comment
	rows2, err := db.Query(`SELECT vcl.email, COALESCE(u.nickname, ''), COALESCE(u.avatar_url, ''), vc.content, COALESCE(vl.title, ''), vc.video_id, vc.id, vcl.created_at
		FROM video_comment_likes vcl
		JOIN video_comments vc ON vcl.comment_id = vc.id
		LEFT JOIN users u ON u.email = vcl.email
		LEFT JOIN video_library vl ON vl.video_id = vc.video_id
		WHERE vc.email = ? AND vcl.email != ?
		ORDER BY vcl.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var m messageItem
			m.Type = "video_comment_like"
			if err := rows2.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.MyContent, &m.TargetTitle, &m.VideoID, &m.CommentID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("video_comment_like_%d_%s", m.CommentID, m.FromEmail)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 3. Video likes: someone liked my video
	rows3, err := db.Query(`SELECT vl2.email, COALESCE(u.nickname, ''), COALESCE(u.avatar_url, ''), COALESCE(vl.title, ''), vl2.video_id, vl2.created_at
		FROM video_likes vl2
		JOIN video_uploads vu ON vl2.video_id = vu.video_id
		LEFT JOIN users u ON u.email = vl2.email
		LEFT JOIN video_library vl ON vl.video_id = vl2.video_id
		WHERE vu.email = ? AND vl2.email != ?
		ORDER BY vl2.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var m messageItem
			m.Type = "video_like"
			if err := rows3.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.TargetTitle, &m.VideoID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("video_like_%s_%s", m.VideoID, m.FromEmail)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 4. Post comment replies: someone replied to my post comment
	rows4, err := db.Query(`SELECT c2.email, c2.nickname, COALESCE(u.avatar_url, ''), c2.content, c.content, COALESCE(p.title, ''), c.post_id, c2.id, c2.created_at
		FROM post_comments c2
		JOIN post_comments c ON c2.parent_id = c.id
		LEFT JOIN users u ON u.email = c2.email
		LEFT JOIN posts p ON p.id = c.post_id
		WHERE c.email = ? AND c2.email != ?
		ORDER BY c2.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var m messageItem
			m.Type = "post_reply"
			if err := rows4.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.Content, &m.MyContent, &m.TargetTitle, &m.PostID, &m.CommentID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("post_reply_%d", m.CommentID)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 4b. Post direct comments: someone commented on my post (top-level)
	rows4b, err := db.Query(`SELECT pc.email, pc.nickname, COALESCE(u.avatar_url, ''), pc.content, COALESCE(p.title, ''), pc.post_id, pc.id, pc.created_at
		FROM post_comments pc
		JOIN posts p ON pc.post_id = p.id
		LEFT JOIN users u ON u.email = pc.email
		WHERE p.email = ? AND pc.email != ? AND pc.parent_id IS NULL
		ORDER BY pc.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows4b.Close()
		for rows4b.Next() {
			var m messageItem
			m.Type = "post_comment"
			if err := rows4b.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.Content, &m.TargetTitle, &m.PostID, &m.CommentID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("post_comment_%d", m.CommentID)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 5. Post comment likes: someone liked my post comment
	rows5, err := db.Query(`SELECT pcl.email, COALESCE(u.nickname, ''), COALESCE(u.avatar_url, ''), pc.content, COALESCE(p.title, ''), pc.post_id, pc.id, pcl.created_at
		FROM post_comment_likes pcl
		JOIN post_comments pc ON pcl.comment_id = pc.id
		LEFT JOIN users u ON u.email = pcl.email
		LEFT JOIN posts p ON p.id = pc.post_id
		WHERE pc.email = ? AND pcl.email != ?
		ORDER BY pcl.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows5.Close()
		for rows5.Next() {
			var m messageItem
			m.Type = "post_comment_like"
			if err := rows5.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.MyContent, &m.TargetTitle, &m.PostID, &m.CommentID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("post_comment_like_%d_%s", m.CommentID, m.FromEmail)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 6. Post likes: someone liked my post
	rows6, err := db.Query(`SELECT pl.email, COALESCE(u.nickname, ''), COALESCE(u.avatar_url, ''), COALESCE(p.title, ''), pl.post_id, pl.created_at
		FROM post_likes pl
		JOIN posts p ON pl.post_id = p.id
		LEFT JOIN users u ON u.email = pl.email
		WHERE p.email = ? AND pl.email != ?
		ORDER BY pl.created_at DESC LIMIT 50`, email, email)
	if err == nil {
		defer rows6.Close()
		for rows6.Next() {
			var m messageItem
			m.Type = "post_like"
			if err := rows6.Scan(&m.FromEmail, &m.FromNickname, &m.FromAvatar, &m.TargetTitle, &m.PostID, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("post_like_%d_%s", m.PostID, m.FromEmail)
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 7. System notifications
	rows7, err := db.Query(`SELECT id, title, content, created_at FROM system_notifications ORDER BY created_at DESC LIMIT 50`)
	if err == nil {
		defer rows7.Close()
		for rows7.Next() {
			var m messageItem
			var nid int64
			m.Type = "system"
			if err := rows7.Scan(&nid, &m.TargetTitle, &m.Content, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("system_%d", nid)
			m.FromNickname = "系统通知"
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 7b. 用户系统消息（如禁言通知）
	rows7b, err := db.Query(`SELECT id, title, content, created_at FROM user_system_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if err == nil {
		defer rows7b.Close()
		for rows7b.Next() {
			var m messageItem
			var nid int64
			m.Type = "user_system"
			if err := rows7b.Scan(&nid, &m.TargetTitle, &m.Content, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("user_system_%d", nid)
			m.FromNickname = "系统"
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 8. Video review messages (per-user)
	rows8, err := db.Query(`SELECT id, video_id, title, result, reason, created_at
		FROM video_review_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if err == nil {
		defer rows8.Close()
		for rows8.Next() {
			var m messageItem
			var rid int64
			var result, reason string
			m.Type = "video_review"
			if err := rows8.Scan(&rid, &m.VideoID, &m.TargetTitle, &result, &reason, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("video_review_%d", rid)
			m.FromNickname = "视频审核"
			m.Content = reason
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 9. Post review messages (per-user)
	rows9, err := db.Query(`SELECT id, post_id, title, result, reason, created_at
		FROM post_review_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if err == nil {
		defer rows9.Close()
		for rows9.Next() {
			var m messageItem
			var rid int64
			var pid int64
			var result, reason string
			m.Type = "post_review"
			if err := rows9.Scan(&rid, &pid, &m.TargetTitle, &result, &reason, &m.CreatedAt); err != nil {
				continue
			}
			m.MsgKey = fmt.Sprintf("post_review_%d", rid)
			m.FromNickname = "帖子审核"
			m.Content = reason
			m.PostID = pid
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// 10. Comment review (deleted) messages: 你在"xxx"发表的言论可能涉及歧视辱骂已被删除
	rows10, err := db.Query(`SELECT id, kind, COALESCE(video_id,''), COALESCE(post_id,0), target_title, reason, created_at
		FROM comment_review_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if err == nil {
		defer rows10.Close()
		for rows10.Next() {
			var m messageItem
			var rid int64
			var kind, videoID string
			var postID int64
			if err := rows10.Scan(&rid, &kind, &videoID, &postID, &m.TargetTitle, &m.Content, &m.CreatedAt); err != nil {
				continue
			}
			m.Type = "comment_review"
			m.MsgKey = fmt.Sprintf("comment_review_%d", rid)
			m.FromNickname = "评论审核"
			if kind == "video" {
				m.VideoID = videoID
			} else {
				m.PostID = postID
			}
			if !deleted[m.MsgKey] {
				messages = append(messages, m)
			}
		}
	}

	// Sort all by created_at DESC
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.After(messages[j].CreatedAt)
	})

	// Limit total
	if len(messages) > 100 {
		messages = messages[:100]
	}
	if messages == nil {
		messages = []messageItem{}
	}

	writeJSON(w, messages)
}

func handleMessagesRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	_, err := db.Exec("UPDATE users SET message_read_at = ? WHERE email = ?", time.Now(), user.Email)
	if err != nil {
		http.Error(w, "failed to update", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleMessagesUnreadCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	email := user.Email

	// Get message_read_at
	var readAt sql.NullTime
	_ = db.QueryRow("SELECT message_read_at FROM users WHERE email = ?", email).Scan(&readAt)
	readTime := time.Time{}
	if readAt.Valid {
		readTime = readAt.Time
	}

	var total int

	var cnt int
	// 1. video comment replies
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_comments c2 JOIN video_comments c ON c2.parent_id = c.id WHERE c.email = ? AND c2.email != ? AND c2.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 1b. video direct comments on my videos
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_comments vc JOIN video_uploads vu ON vc.video_id = vu.video_id WHERE vu.email = ? AND vc.email != ? AND vc.parent_id IS NULL AND vc.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 2. video comment likes
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_comment_likes vcl JOIN video_comments vc ON vcl.comment_id = vc.id WHERE vc.email = ? AND vcl.email != ? AND vcl.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 3. video likes
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_likes vl JOIN video_uploads vu ON vl.video_id = vu.video_id WHERE vu.email = ? AND vl.email != ? AND vl.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 4. post comment replies
	if err := db.QueryRow(`SELECT COUNT(*) FROM post_comments c2 JOIN post_comments c ON c2.parent_id = c.id WHERE c.email = ? AND c2.email != ? AND c2.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 4b. post direct comments on my posts
	if err := db.QueryRow(`SELECT COUNT(*) FROM post_comments pc JOIN posts p ON pc.post_id = p.id WHERE p.email = ? AND pc.email != ? AND pc.parent_id IS NULL AND pc.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 5. post comment likes
	if err := db.QueryRow(`SELECT COUNT(*) FROM post_comment_likes pcl JOIN post_comments pc ON pcl.comment_id = pc.id WHERE pc.email = ? AND pcl.email != ? AND pcl.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 6. post likes
	if err := db.QueryRow(`SELECT COUNT(*) FROM post_likes pl JOIN posts p ON pl.post_id = p.id WHERE p.email = ? AND pl.email != ? AND pl.created_at > ?`, email, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 7. system notifications
	if err := db.QueryRow(`SELECT COUNT(*) FROM system_notifications WHERE created_at > ?`, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 8. video review messages
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_review_messages WHERE email = ? AND created_at > ?`, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 9. post review messages
	if err := db.QueryRow(`SELECT COUNT(*) FROM post_review_messages WHERE email = ? AND created_at > ?`, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}
	// 10. comment review (deleted) messages
	if err := db.QueryRow(`SELECT COUNT(*) FROM comment_review_messages WHERE email = ? AND created_at > ?`, email, readTime).Scan(&cnt); err == nil {
		total += cnt
	}

	writeJSON(w, map[string]any{"count": total})
}

func handleMessagesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		MsgKey string `json:"msgKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.MsgKey == "" {
		http.Error(w, "msgKey required", http.StatusBadRequest)
		return
	}
	now := time.Now()
	_, _ = db.Exec("INSERT IGNORE INTO deleted_messages (email, msg_key, created_at) VALUES (?, ?, ?)", user.Email, req.MsgKey, now)
	_, _ = db.Exec("UPDATE users SET message_read_at = ? WHERE email = ?", now, user.Email)
	writeJSON(w, map[string]any{"status": "ok"})
}

func handleMessagesDeleteAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	// Get all current message keys and insert them as deleted
	// Simpler approach: mark read_at to far future so nothing shows as unread, and store all keys
	// But actually, we need to fetch all current messages and mark them as deleted
	// For efficiency, let's just call the same logic to get keys
	email := user.Email
	var keys []string

	// Collect all current msg keys (same queries as handleMessages but only select keys)
	r1, _ := db.Query(`SELECT c2.id FROM video_comments c2 JOIN video_comments c ON c2.parent_id = c.id WHERE c.email = ? AND c2.email != ? ORDER BY c2.created_at DESC LIMIT 50`, email, email)
	if r1 != nil {
		defer r1.Close()
		for r1.Next() {
			var id int64
			if r1.Scan(&id) == nil {
				keys = append(keys, fmt.Sprintf("video_reply_%d", id))
			}
		}
	}
	r1b, _ := db.Query(`SELECT vc.id FROM video_comments vc JOIN video_uploads vu ON vc.video_id = vu.video_id WHERE vu.email = ? AND vc.email != ? AND vc.parent_id IS NULL ORDER BY vc.created_at DESC LIMIT 50`, email, email)
	if r1b != nil {
		defer r1b.Close()
		for r1b.Next() {
			var id int64
			if r1b.Scan(&id) == nil {
				keys = append(keys, fmt.Sprintf("video_comment_%d", id))
			}
		}
	}
	r2, _ := db.Query(`SELECT vcl.comment_id, vcl.email FROM video_comment_likes vcl JOIN video_comments vc ON vcl.comment_id = vc.id WHERE vc.email = ? AND vcl.email != ? ORDER BY vcl.created_at DESC LIMIT 50`, email, email)
	if r2 != nil {
		defer r2.Close()
		for r2.Next() {
			var cid int64
			var fe string
			if r2.Scan(&cid, &fe) == nil {
				keys = append(keys, fmt.Sprintf("video_comment_like_%d_%s", cid, fe))
			}
		}
	}
	r3, _ := db.Query(`SELECT vl2.video_id, vl2.email FROM video_likes vl2 JOIN video_uploads vu ON vl2.video_id = vu.video_id WHERE vu.email = ? AND vl2.email != ? ORDER BY vl2.created_at DESC LIMIT 50`, email, email)
	if r3 != nil {
		defer r3.Close()
		for r3.Next() {
			var vid, fe string
			if r3.Scan(&vid, &fe) == nil {
				keys = append(keys, fmt.Sprintf("video_like_%s_%s", vid, fe))
			}
		}
	}
	r4, _ := db.Query(`SELECT c2.id FROM post_comments c2 JOIN post_comments c ON c2.parent_id = c.id WHERE c.email = ? AND c2.email != ? ORDER BY c2.created_at DESC LIMIT 50`, email, email)
	if r4 != nil {
		defer r4.Close()
		for r4.Next() {
			var id int64
			if r4.Scan(&id) == nil {
				keys = append(keys, fmt.Sprintf("post_reply_%d", id))
			}
		}
	}
	r4b, _ := db.Query(`SELECT pc.id FROM post_comments pc JOIN posts p ON pc.post_id = p.id WHERE p.email = ? AND pc.email != ? AND pc.parent_id IS NULL ORDER BY pc.created_at DESC LIMIT 50`, email, email)
	if r4b != nil {
		defer r4b.Close()
		for r4b.Next() {
			var id int64
			if r4b.Scan(&id) == nil {
				keys = append(keys, fmt.Sprintf("post_comment_%d", id))
			}
		}
	}
	r5, _ := db.Query(`SELECT pcl.comment_id, pcl.email FROM post_comment_likes pcl JOIN post_comments pc ON pcl.comment_id = pc.id WHERE pc.email = ? AND pcl.email != ? ORDER BY pcl.created_at DESC LIMIT 50`, email, email)
	if r5 != nil {
		defer r5.Close()
		for r5.Next() {
			var cid int64
			var fe string
			if r5.Scan(&cid, &fe) == nil {
				keys = append(keys, fmt.Sprintf("post_comment_like_%d_%s", cid, fe))
			}
		}
	}
	r6, _ := db.Query(`SELECT pl.post_id, pl.email FROM post_likes pl JOIN posts p ON pl.post_id = p.id WHERE p.email = ? AND pl.email != ? ORDER BY pl.created_at DESC LIMIT 50`, email, email)
	if r6 != nil {
		defer r6.Close()
		for r6.Next() {
			var pid int64
			var fe string
			if r6.Scan(&pid, &fe) == nil {
				keys = append(keys, fmt.Sprintf("post_like_%d_%s", pid, fe))
			}
		}
	}
	r7, _ := db.Query(`SELECT id FROM system_notifications ORDER BY created_at DESC LIMIT 50`)
	if r7 != nil {
		defer r7.Close()
		for r7.Next() {
			var nid int64
			if r7.Scan(&nid) == nil {
				keys = append(keys, fmt.Sprintf("system_%d", nid))
			}
		}
	}
	r8, _ := db.Query(`SELECT id FROM video_review_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if r8 != nil {
		defer r8.Close()
		for r8.Next() {
			var rid int64
			if r8.Scan(&rid) == nil {
				keys = append(keys, fmt.Sprintf("video_review_%d", rid))
			}
		}
	}
	r9, _ := db.Query(`SELECT id FROM post_review_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if r9 != nil {
		defer r9.Close()
		for r9.Next() {
			var rid int64
			if r9.Scan(&rid) == nil {
				keys = append(keys, fmt.Sprintf("post_review_%d", rid))
			}
		}
	}
	r10, _ := db.Query(`SELECT id FROM comment_review_messages WHERE email = ? ORDER BY created_at DESC LIMIT 50`, email)
	if r10 != nil {
		defer r10.Close()
		for r10.Next() {
			var rid int64
			if r10.Scan(&rid) == nil {
				keys = append(keys, fmt.Sprintf("comment_review_%d", rid))
			}
		}
	}

	now := time.Now()
	for _, key := range keys {
		_, _ = db.Exec("INSERT IGNORE INTO deleted_messages (email, msg_key, created_at) VALUES (?, ?, ?)", email, key, now)
	}
	_, _ = db.Exec("UPDATE users SET message_read_at = ? WHERE email = ?", now, email)

	writeJSON(w, map[string]any{"status": "ok"})
}

func handleSystemNotifications(w http.ResponseWriter, r *http.Request) {
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

func handlePostView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := getAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		PostID int64 `json:"postId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.PostID <= 0 {
		http.Error(w, "postId required", http.StatusBadRequest)
		return
	}
	var postExists int
	if err := db.QueryRow("SELECT 1 FROM posts WHERE id = ?", req.PostID).Scan(&postExists); err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	_, err := db.Exec("INSERT IGNORE INTO post_views (post_id, email, created_at) VALUES (?, ?, ?)", req.PostID, user.Email, time.Now())
	if err != nil {
		http.Error(w, "failed to view", http.StatusInternalServerError)
		return
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM post_views WHERE post_id = ?", req.PostID).Scan(&count); err != nil {
		http.Error(w, "failed to count views", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "viewCount": count})
}
