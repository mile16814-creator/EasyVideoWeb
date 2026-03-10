# EasyVideoWeb Project Documentation

This document describes configuration, i18n, deployment, and features of **EasyVideoWeb** for development and operations.

---
<img width="1961" height="1955" alt="屏幕截图 2026-03-03 034940" src="https://github.com/user-attachments/assets/8af70942-9866-4dae-a767-2b6d230fe37a" />
<img width="2835" height="1744" alt="屏幕截图 2026-03-02 222200" src="https://github.com/user-attachments/assets/7dd55248-bdd1-4b13-8808-7ce1f484fa54" />
<img width="2667" height="1283" alt="屏幕截图 2026-03-03 040452" src="https://github.com/user-attachments/assets/b3696ed5-9b0a-4c70-8263-7d423ca7614c" />


## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Directory Structure](#2-directory-structure)
3. [Environment Requirements](#3-environment-requirements)
4. [Configuration Files](#4-configuration-files)
5. [Virtual Environment and Python](#5-virtual-environment-and-python)
6. [AI Review Pipeline](#6-ai-review-pipeline)
7. [Database Configuration (MySQL)](#7-database-configuration-mysql)
8. [Email Configuration (SMTP)](#8-email-configuration-smtp)
9. [JWT and Authentication](#9-jwt-and-authentication)
10. [Internationalization (i18n)](#10-internationalization-i18n)
11. [Pagination and Display](#11-pagination-and-display)
12. [Build and Run](#12-build-and-run)
13. [Main Site API List](#13-main-site-api-list)
14. [Admin APIs and Features](#14-admin-apis-and-features)
15. [Frontend Pages and Routes](#15-frontend-pages-and-routes)
16. [FAQ and Troubleshooting](#16-faq-and-troubleshooting)

---

## 1. Project Overview

- **Project Name**: EasyVideoWeb
- **Tech Stack**: Go backend + static frontend (Vue 3 + vanilla JS) + MySQL
- **Key Features**: Video/post publishing, comments, likes, favorites, messages, user profile, review pipeline, admin console
- **Modules**:
  - **Main site**: `main.go` + `main_extension.go`, default port **8080**
  - **Admin**: `admin/admin_server.go`, default port **8081**

---

## 2. Directory Structure

```
E:\boke\ (project root)
├── main.go                 # main site entry, routes, business logic
├── main_extension.go       # main site extensions (messages, review, etc.)
├── go.mod                  # Go module (module easyvideoweb)
├── app.config.json         # app config: venv, pagination
├── mysql.local.json        # MySQL connection (fill in, do not commit secrets)
├── smtp.local.json         # SMTP config (fill in)
├── jwt.local.json          # JWT secret (copy from jwt.local.example.json)
├── mysql.local.example.json # MySQL config template (placeholders)
├── smtp.local.example.json  # SMTP config template (placeholders)
├── jwt.local.example.json   # JWT config template
├── storage/                 # media and uploads
│   ├── videos/
│   ├── avatars/
│   ├── banners/
│   └── posters/
├── uploads/                # post images and uploads
├── web/                     # frontend static assets
│   ├── index.html
│   ├── posts.html
│   ├── user.html
│   ├── player.html
│   ├── creator.html
│   ├── post_create.html
│   ├── post_detail.html
│   ├── messages.html
│   ├── register.html
│   ├── styles.css
│   ├── app.js
│   ├── posts.js
│   ├── player.js
│   └── language/            # i18n JSON
│       ├── zh-CN.json       # Chinese UI
│       ├── en-US.json       # English UI
│       ├── ru-RU.json       # Russian UI
│       ├── backend-zh-CN.json  # backend error messages (CN)
│       ├── backend-en-US.json
│       └── backend-ru-RU.json
├── admin/
│   └── admin_server.go     # admin service (standalone executable)
├── flash/                   # Python virtual environment (optional, see app.config.json)
├── requirements_review.txt  # Python review dependencies
└── DOCUMENTATION.md         # this document
```

---

## 3. Environment Requirements

| Item       | Requirement |
|------------|-------------|
| Go         | 1.22+ |
| MySQL      | 5.7+ / 8.x, database must exist or be created by the program |
| FFmpeg     | Required for transcoding, thumbnails, metadata; must be in PATH |
| Browser    | ES6, Vue 3 (CDN) |
| tiptop     | Review dependency, install inside the virtual environment |
| Python     | Required only when AI review is enabled, see [Virtual Environment](#5-virtual-environment-and-python) |

---

## 4. Configuration Files

### 4.1 Overview

| File | Purpose | Required | Commit to Repo |
|------|--------|----------|----------------|
| `app.config.json` | venv switch, paths, pagination | Optional (defaults exist) | Recommended |
| `mysql.local.json` | MySQL connection | **Required** | **No** (use example) |
| `smtp.local.json` | SMTP config (registration/code) | Required for mail | **No** |
| `jwt.local.json` | JWT signing secret | **Required** | **No** |
| `mysql.local.example.json` | MySQL template | Reference | Yes |
| `smtp.local.example.json` | SMTP template | Reference | Yes |
| `jwt.local.example.json` | JWT template | Reference | Yes |

**Environment variables can override config** (main site):  
`MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASS` / `MYSQL_PASSWORD`, `MYSQL_DATABASE`.

---

### 4.2 app.config.json (App Config)

Path: project root `app.config.json`.

```json
{
  "publish_quota": {
    "video_base": 3,
    "post_base": 3,
    "likes_per_bonus": 20,
    "comment": "Initial publish quota per user; +1 for each likes_per_bonus likes."
  },
  "venv": {
    "use_venv": true,
    "venv_folder": "flash",
    "python_subpath": "Scripts/python.exe",
    "pip_subpath": "Scripts/pip.exe",
    "comment": "Set use_venv to false to use system Python. On Linux/Mac use bin/python and bin/pip instead of Scripts/python.exe and Scripts/pip.exe."
  },
  "pagination": {
    "videos_per_page": 12,
    "posts_per_page": 10
  },
  "server": {
    "main_listen_addr": ":8080",
    "main_base_url": "http://localhost:8080",
    "admin_listen_addr": ":8081",
    "admin_base_url": "http://localhost:8081"
  }
}
```

- **publish_quota** (publish quota, read on startup)  
  - **video_base**: base video publish quota, default 3.  
  - **post_base**: base post publish quota, default 3.  
  - **likes_per_bonus**: +1 publish chance per N likes, default 20.  
  - If `app.config.json` is missing or `publish_quota` is missing, default values (3/3/20) are used.

- **venv.use_venv**  
  - `true`: use Python from the venv (for review scripts).  
  - `false`: use system `python` and skip venv checks.

- **venv.venv_folder**  
  venv folder name (relative to project root), e.g. `flash`.

- **venv.python_subpath** / **venv.pip_subpath**  
  paths relative to `venv_folder`:  
  - Windows: `Scripts/python.exe`, `Scripts/pip.exe`  
  - Linux/macOS: `bin/python`, `bin/pip`

- **pagination.videos_per_page**  
  Videos per page setting; the frontend reads it dynamically from `/api/app-config`.

- **pagination.posts_per_page**  
  Posts per page setting; the frontend reads it dynamically from `/api/app-config`.

- **server.main_listen_addr** / **server.admin_listen_addr**  
  Listen addresses for the main site and admin site, default `:8080` and `:8081`.

- **server.main_base_url** / **server.admin_base_url**  
  Public URLs used for startup output and admin review/report jump links.

**Defaults when missing**:  
- venv defaults to `use_venv: true`, `venv_folder: "flash"`, Windows paths.  
- Pagination and listen addresses fall back to built-in defaults.

---

## 5. Virtual Environment and Python

### 5.1 When Python Is Needed

- If video/text/image reviews are done via Python scripts (e.g., `review_video.py`, `review_text.py`, `review_image.py`), Python is required.
- If you do not need reviews, set `use_venv: false` in `app.config.json` and the main site will not depend on a venv.

### 5.2 Using a Virtual Environment (Recommended)

1. Create a venv in the project root, for example:
   ```bash
   python -m venv flash
   ```
2. Configure `app.config.json`:
   - `use_venv: true`
   - `venv_folder: "flash"`
   - `python_subpath`: Windows `Scripts/python.exe`, Linux/Mac `bin/python`
   - `pip_subpath`: Windows `Scripts/pip.exe`, Linux/Mac `bin/pip`
3. On startup, the main site will:
   - Check whether `venv_folder` + `python_subpath` exists
   - If `requirements_review.txt` exists, install dependencies via venv pip
   - Use this Python interpreter for review scripts

### 5.3 Using System Python

- Set `"use_venv": false` in `app.config.json`.
- The main site uses `python` in PATH and skips venv checks.

### 5.4 Linux / macOS Example

```json
{
  "venv": {
    "use_venv": true,
    "venv_folder": "flash",
    "python_subpath": "bin/python",
    "pip_subpath": "bin/pip"
  },
  "pagination": { "videos_per_page": 12, "posts_per_page": 10 }
}
```

---

## 6. AI Review Pipeline

All **videos, posts, and comments** enter the AI review pipeline after publishing. The Go main site invokes Python scripts to detect violations; approved content is shown, rejected content is blocked or deleted, and manual review is optional for some content.

### 6.1 Overview

| Content | Trigger | Method | Approved | Rejected | Manual Review |
|---------|---------|--------|----------|----------|---------------|
| **Video** | After upload is stored | Async queue, `review_video.py` | Status `approved`, visible on homepage | `rejected_violence` / `rejected_nsfw`, author can request review | Yes |
| **Post** | After publishing | Sync: text → cover/body images | Status `approved` | `rejected_abuse` / `rejected_nsfw` / `rejected_violence`, author can request review | Yes |
| **Comment** | On submit | Text only, `review_text.py` | Status `approved` | **Deleted immediately**, author notified | No |

- **Video review**: max 2 concurrent workers (semaphore), 10-minute timeout; script errors/timeouts default to approve to avoid blocking.
- **Post review**: text timeout 2 minutes, image timeout 3 minutes; any failure rejects the post.
- **Comment review**: text only, timeout 2 minutes; on failure, delete comment and insert `comment_review_messages`.

### 6.2 Video Review (review_video.py)

- **Entry**: after upload, Go sets `video_library.review_status` to `pending` and calls `enqueueVideoReview(videoID, filePath, authorEmail, title, description)`.
- **Flow**: first review text (title + description) via `review_text.py`; if rejected, status becomes `rejected_abuse` and user is notified. If passed, review video frames.
- **Script**: `review_video.py` in project root, usage `python review_video.py <video file>`.  
- **Output**: one JSON line to stdout, e.g. `{"approved": true}` or `{"approved": false, "reject_reason": "violence"}`; progress/logs to stderr.
- **Process**:
  1. **Frame sampling**: OpenCV reads the video, roughly 2 fps (configurable).
  2. **Violence detection**:
     - Model: VGG19 + LSTM, 30 frames per clip, 160×160 input. Weights: `Real-Time-Violence-Detection-in-Video-*/mamonbest947oscombo-drive.hdfs` (provide or train).
     - Any clip probability ≥ 0.7 → reject with `reject_reason: "violence"`.
  3. **NSFW detection**:
     - Model: Caffe ResNet-50, files under `open_nsfw/nsfw_model/` (`deploy.prototxt`, `resnet_50_1by2_nsfw.caffemodel`).
     - Frame-wise 224×224 inference. Avg NSFW ≥ 0.7 or 3+ consecutive frames ≥ 0.7 → reject with `reject_reason: "nsfw"`.
- **Result**: Go parses stdout JSON and calls `setReviewResult(videoID, status, ...)` to update `video_library.review_status` (`approved` / `rejected_abuse` / `rejected_violence` / `rejected_nsfw`), and inserts into `video_review_messages`.
- **Dependencies**: Python 3, OpenCV, TensorFlow/Keras (violence), Caffe/OpenCV DNN (NSFW). If model files are missing, that check is skipped and treated as approved.

### 6.3 Post Review (Text + Images)

- **Entry**: after publish, Go calls `enqueuePostReview(postID, contentText, contentHTML, imagePath, authorEmail, authorNickname, title)` in a goroutine (sync execution, no queue).
- **Steps**:
  1. **Text review**: write text to temp file, run `review_text.py <temp>`.
  2. **Image review**: parse `/uploads/` image paths from cover/body HTML, run `review_image.py <abs_path>` per image.
- **review_text.py**:
  - Model: ModelScope text classification `damo/nlp_structbert_abuse-detect_chinese-tiny` (safe vs abuse).
  - Split long text every 300 chars; any segment flagged as abuse → `passed: false, reject_reason: "abuse"`.
  - If modelscope missing or script error, default to pass.
- **review_image.py**:
  - Must be in project root next to `review_video.py`; outputs JSON with `approved` and `reject_reason` (`nsfw`, `violence`).
  - Any failed image sets post status to `rejected_nsfw` or `rejected_violence`, and writes `post_review_messages`.
- **Result**: all pass → `setReviewResultPost(..., "approved", ...)`; otherwise set `rejected_abuse` / `rejected_nsfw` / `rejected_violence` and notify author.

### 6.4 Comment Review (Text Only)

- **Entry**: on comment submit, Go inserts with `review_status = pending`, then async calls `runCommentReview(kind, commentID, videoID|postID, content, authorEmail, targetTitle)`.
- **Logic**:
  - Run `review_text.py` (same model).
  - **Pass**: update `video_comments` / `post_comments` to `approved`.
  - **Fail**: **delete the comment** (and related likes), insert `comment_review_messages` to notify the user.
- **Difference**: comments do not support manual review; failed comments are deleted.

### 6.5 Manual Review (Videos and Posts)

- **User side**: in profile → videos/posts list, items in “rejected” or “takedown” can request manual review; Go inserts into `manual_video_reviews` / `manual_post_reviews` with status `pending`.
- **Admin**: “Video Manual Review” / “Post Manual Review” lists show pending items; admins can approve or reject with reviewer/notes. Approvals set `review_status` to `approved`.
- **Preview access**: admin review links include `adminReviewToken`; main site validates `manual_review_access_tokens` to allow preview of non-public content.

### 6.6 Review Tables and Messages

- **Video**: `video_library.review_status`, `video_review_messages` (notify author per review run).
- **Post**: `posts.review_status`, `post_review_messages`.
- **Comment**: `video_comments.review_status` / `post_comments.review_status`, `comment_review_messages` (only on deletion).
- **Manual review**: `manual_video_reviews`, `manual_review_access_tokens`, `manual_post_reviews`, `manual_post_review_access_tokens`.

### 6.7 Dependencies and Runtime Summary

- **Python**: determined by venv config in `app.config.json` or system `python`; scripts must import `cv2`, `tensorflow`/`tf_keras` (video), `modelscope` (text), and Caffe model files (NSFW).
- **Models and weights**:
  - Violence: `Real-Time-Violence-Detection-in-Video-*/mamonbest947oscombo-drive.hdfs`
  - NSFW: `open_nsfw/nsfw_model/deploy.prototxt`, `resnet_50_1by2_nsfw.caffemodel`
  - Text abuse: ModelScope downloads `damo/nlp_structbert_abuse-detect_chinese-tiny`
- **Script errors/timeouts**: video review defaults to approve; post/comment text defaults to pass; image failures may be skipped or treated as reject depending on implementation.
- **Disable AI review**: set `use_venv: false` and ensure publish logic does not depend on review results; or keep venv and replace scripts with always-approve stubs.

---

## 7. Database Configuration (MySQL)

### 7.1 Config File

- **Actual**: `mysql.local.json` (do not commit).  
- **Template**: `mysql.local.example.json` (committable).

### 7.2 mysql.local.example.json and Placeholders

```json
{
  "host": "127.0.0.1",
  "port": "3306",
  "user": "your_mysql_user",
  "pass": "your_mysql_password",
  "database": "easyvideoweb"
}
```

Deployment steps:

1. Copy `mysql.local.example.json` to `mysql.local.json`.
2. Replace `your_mysql_user` and `your_mysql_password` with real credentials.
3. Set `database` to the target schema (default `easyvideoweb`).
4. Both main site and admin read `mysql.local.json`. If `pass` is empty or still the placeholder, the main site exits with an error.

### 7.3 Environment Variables (Main Site)

These variables override `mysql.local.json`:

- `MYSQL_HOST`
- `MYSQL_PORT`
- `MYSQL_USER`
- `MYSQL_PASS` or `MYSQL_PASSWORD`
- `MYSQL_DATABASE`

---

## 8. Email Configuration (SMTP)

### 8.1 Purpose

- Registration codes, login codes, change-email verification, etc.
- If SMTP is not configured, related APIs return “mail service not configured”.

### 8.2 Config Files

- **Actual**: `smtp.local.json` (do not commit).  
- **Template**: `smtp.local.example.json`.

Example (placeholders):

```json
{
  "host": "smtp.example.com",
  "port": "587",
  "user": "your_smtp_user",
  "pass": "your_smtp_password"
}
```

If `user` or `pass` is still `your_smtp_user` / `your_smtp_password`, the service is treated as unconfigured and no email is sent.

### 8.3 Common SMTP Examples

- Gmail: host `smtp.gmail.com`, port 587, use App Passwords.  
- QQ Mail: host `smtp.qq.com`, port 465 or 587, enable SMTP and use authorization code.  
- Self-hosted: use actual host/port and open firewall as needed.

---

## 9. JWT and Authentication

### 9.1 jwt.local.json

- The main site reads `secret` from `jwt.local.json` to sign and verify tokens.
- If missing or empty, a temporary secret is generated on startup (changes on restart, not recommended for production).

### 9.2 Example and Security

- Copy `jwt.local.example.json` to `jwt.local.json`.  
- Replace `secret` with a long random string, keep it private, and do not commit it.  
- Example: `"secret": "easyvideoweb_jwt_secret_change_me"` must be changed in production.

---

## 10. Internationalization (i18n)

### 10.1 Supported Languages

- **Main site UI**: zh-CN, en-US, ru-RU.  
- **Backend API errors**: same languages via backend language files and request headers.

### 10.2 Main Site Language Files (Frontend)

Path: `web/language/`.

| File | Description |
|------|-------------|
| `zh-CN.json` | Chinese UI strings (common, user, index, player, messages, posts, post_detail, creator, post_create, register_page, etc.) |
| `en-US.json` | English UI strings, same structure as zh-CN |
| `ru-RU.json` | Russian UI strings, same structure as zh-CN |

Frontend loads via `GET /language/{locale}.json`, e.g. `/language/zh-CN.json`.  
Keys are hierarchical, e.g. `common.header.videoHome`, `user.settings.language`.

### 10.3 Backend API Language Files

Path: `web/language/`.

| File | Description |
|------|-------------|
| `backend-zh-CN.json` | Chinese API errors/prompts (snake_case keys like `failed_to_load_categories`) |
| `backend-en-US.json` | English |
| `backend-ru-RU.json` | Russian |

Both main site and admin load these files and return errors based on request headers or admin locale.

### 10.4 Default Language and Priority

- **Main site**  
  - Prefer `evw_locale` in **localStorage** (set via profile → settings → language).  
  - If not set, call **`GET /api/locale`**; server determines locale based on **Accept-Language** (zh-CN / en-US / ru-RU). Frontend stores `evw_locale` and loads the pack.  
  - IP-based locale is not implemented.

- **Admin**  
  - Locale stored in **localStorage** as `evw_admin_locale`.  
  - All requests include **`X-Locale`**, and backend uses it for localized errors.

### 10.5 Switch Language (Main Site)

- After login, go to **Profile** → **Settings**.  
- Select Simplified Chinese / English / Русский.  
- The selection updates `evw_locale` and reloads the language pack without re-login.

### 10.6 Switch Language (Admin)

- Use the top “Language” selector on the admin page.  
- It writes `evw_admin_locale`; subsequent API calls include `X-Locale`.

### 10.7 How Backend Chooses Language

- Main site: `getLocaleFromRequest(r)` checks `X-Locale`, otherwise parses `Accept-Language` and maps to zh-CN / en-US / ru-RU.  
- Admin: only uses `X-Locale` (set by frontend).  
- Errors come from backend JSON via `writeBackendError(w, r, key, code)` / `writeAdminError(w, r, key, code)`.

---

## 11. Pagination and Display

### 11.1 Configuration Source

- **Frontend scripts**: `state.pageSize` in `web/app.js` (homepage videos), `state.postPageSize` in `web/posts.js` (post list).
- **Backend**: no pagination config API currently; `/api/videos` and `/api/posts` return full lists, and the frontend paginates.

### 11.2 Behavior

- Home (index): videos per page = `state.pageSize` (default 12).  
- Posts list: posts per page = `state.postPageSize` (default 10).  
- To change counts, update `web/app.js` and `web/posts.js`, then redeploy static assets.

### 11.3 Monthly Rankings

- **Videos**: homepage “Top Videos This Month” from `GET /api/rankings/videos`, only content after the 1st 00:00 of the month, ranked by likes/comments score, top 10.
- **Posts**: posts page “Top Posts” from `GET /api/rankings/posts`, same rules.
- Rankings are computed once at process start and refreshed every **12 hours**.

---

## 12. Build and Run

The build produces a **Go web server binary** and does not include videos or static assets. Media is served from `storage/` and `uploads/` at runtime.

### 12.1 Main Site

**Run directly in development (recommended):**

```bash
# go to project root
cd /path/to/boke

# run without building
go run .
```

Default listen: **http://localhost:8080**.

### 12.2 Admin

**Run directly in development:**

```bash
cd /path/to/boke/admin
go run .
```

Admin listens on **http://localhost:8081**.  
It reads `mysql.local.json` from the **current working directory**. If you run from `admin/`, copy `mysql.local.json` into `admin/`, or run `go run ./admin` from project root to share the same config.

### 12.3 First Deployment Checklist

1. Copy and fill `mysql.local.json` (avoid placeholder passwords).
2. Copy and fill `jwt.local.json` (replace secret for production).
3. If email is needed: copy and fill `smtp.local.json`.
4. For AI review: configure `app.config.json` and prepare the venv.
5. Install **FFmpeg** and add it to PATH.
6. Ensure MySQL schema exists (or let the program create tables).
7. Start main site first, then admin as needed.

---

## 13. Main Site API List

Selected key APIs for reference:

| Method | Path | Description |
|------|------|------|
| GET | `/api/categories` | Video categories |
| GET | `/api/post-categories` | Post categories |
| GET | `/api/videos` | Video list (query: category, q) |
| GET | `/api/posts` | Post list (query: category, q) |
| GET | `/api/rankings/videos` | Monthly top videos (refresh every 12h) |
| GET | `/api/rankings/posts` | Monthly top posts (refresh every 12h) |
| GET | `/api/videos/{id}` | Video detail |
| GET | `/api/posts/{id}` | Post detail |
| GET | `/api/locale` | Suggested locale by Accept-Language |
| POST | `/api/send-code` | Send verification code (registration) |
| POST | `/api/verify-code` | Verify code and register |
| POST | `/api/login` | Password login |
| POST | `/api/login-code/send` | Send login code |
| POST | `/api/login-code/verify` | Verify login code |
| GET | `/api/profile` | Current user profile (auth) |
| GET | `/api/users/profile` | Public profile (query: email) |
| POST | `/api/profile/nickname` | Update nickname |
| POST | `/api/profile/password` | Update/set password |
| POST | `/api/profile/avatar` | Upload avatar |
| POST | `/api/profile/banner` | Upload banner |
| POST | `/api/profile/notice` | Update notice |
| POST | `/api/profile/motto` | Update motto |
| GET | `/api/profile/favorites` | User video favorites |
| GET | `/api/profile/post-favorites` | User post favorites |
| POST | `/api/creator/upload` | Upload video |
| GET | `/api/creator/publish-quota` | Publish quota |
| POST | `/api/change-email/send` | Send change-email code |
| POST | `/api/change-email/verify` | Verify and change email |
| GET | `/api/messages` | Message list |
| POST | `/api/messages/read` | Mark messages read |
| GET | `/api/messages/unread-count` | Unread count |
| POST | `/api/videos/favorite` | Video favorite/unfavorite |
| POST | `/api/videos/like` | Video like/unlike |
| POST | `/api/videos/view` | Record view |
| POST | `/api/posts/favorite` | Post favorite/unfavorite |
| POST | `/api/posts/like` | Post like |
| POST | `/api/posts/view` | Record post view |
| GET/POST/DELETE | `/api/videos/{id}/comments` etc | Comments, likes, delete (see main.go) |
| GET | `/api/homepage-posters` | Homepage carousel |
| GET | `/api/system-notifications` | System notifications |
| GET | `/api/user-punishment` | Current user ban/mute status |

For authenticated APIs, include `Authorization: Bearer <token>`.  
Some endpoints return localized errors based on `X-Locale` or `Accept-Language` (see [i18n](#10-internationalization-i18n)).

---

## 14. Admin APIs and Features

- Admin runs as a standalone service, default port **8081**, HTML embedded in `admin_server.go`.  
- The top language selector (zh-CN / en-US / ru-RU) sets `X-Locale` for localized errors.

Main features (paths may differ; check code):

- CRUD for video categories and post categories.
- Create and list system notifications.
- Manual review queues for videos/posts.
- Report review handling.
- Homepage poster upload/management.
- Bulk video upload.
- Video/post takedown.
- User ban, mute, pardon.
- Search: videos, posts, users.

Error messages are loaded from `web/language/backend-*.json` and share the same key set as the main site.

---

## 15. Frontend Pages and Routes

- The main site is a hybrid of static HTML pages with JS calling APIs.  
- Language packs load from `/language/{locale}.json`, locale comes from `evw_locale` or `/api/locale`.

| Path/File | Description |
|-----------|-------------|
| `/`, `index.html` | Home: video list, categories, carousel, pagination |
| `/posts.html` | Post list, categories, pagination |
| `/user.html` | User profile (self or others), settings, language |
| `/player.html` | Video player |
| `/creator.html` | Video upload |
| `/post_create.html` | Create post |
| `/post_detail.html` | Post detail |
| `/messages.html` | Message center |
| `/register.html` | Register |

The language selector in **Profile → Settings** stores `evw_locale` and affects all pages.

---

## 16. FAQ and Troubleshooting

### 16.1 Main site startup error: mysql pass missing or not configured

- `mysql.local.json` is missing or `pass` still has the placeholder `your_mysql_password`.  
- Fix: copy `mysql.local.example.json` to `mysql.local.json` and set the real MySQL password.

### 16.2 “Mail service not configured” on registration/code/change-email

- `smtp.local.json` is missing or `user`/`pass` still `your_smtp_user`/`your_smtp_password`.  
- Fix: copy `smtp.local.example.json` to `smtp.local.json` and fill valid SMTP credentials.

### 16.3 Video upload or processing fails

- **FFmpeg** may be missing or not in PATH. Install FFmpeg and ensure `ffmpeg` (and `ffprobe` if used) is available.  
- Check main site logs for FFmpeg errors.

### 16.4 Wrong items per page on home/posts

- Check `state.pageSize` in `web/app.js` and `state.postPageSize` in `web/posts.js`.  
- Redeploy static assets after changes (restarting backend is not enough).

### 16.5 Language not switching or wrong language

- Main site: ensure `evw_locale` in localStorage is zh-CN / en-US / ru-RU; clear cache or reselect in settings.  
- Admin: ensure `evw_admin_locale` matches the top selector, and requests include `X-Locale`.  
- Check `web/language/` for `{locale}.json` and `backend-{locale}.json`.

### 16.6 Python review errors

- If review is not needed: set `use_venv: false` in `app.config.json`.  
- If needed: ensure `venv_folder`, `python_subpath`, `pip_subpath` are correct, venv exists, and `requirements_review.txt` is installed.

### 16.7 Admin cannot connect to database

- Admin and main site share `mysql.local.json` (resolved relative to the current working directory or executable).  
- Ensure admin runtime can find `mysql.local.json` and DB is reachable.

---

## Appendix: Branding and Naming

- Project, UI, and docs use **EasyVideoWeb** consistently (formerly boke).
- Default database name is **easyvideoweb**; placeholders in configs use this name.
- Some legacy CSS class names remain (e.g. `boke-header`), but visible branding is EasyVideoWeb.

---

*This document reflects the current implementation. Update it when the code changes.*
