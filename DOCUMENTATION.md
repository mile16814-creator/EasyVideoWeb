# EasyVideoWeb 项目说明文档

本文档为 **EasyVideoWeb** 的配置、多语言、部署与功能说明，供开发与运维参考。

---

## 目录

1. [项目概述](#1-项目概述)
2. [目录结构](#2-目录结构)
3. [环境要求](#3-环境要求)
4. [配置文件详解](#4-配置文件详解)
5. [虚拟环境与 Python 配置](#5-虚拟环境与-python-配置)
6. [AI 全流程审核](#6-ai-全流程审核)
7. [数据库配置 (MySQL)](#7-数据库配置-mysql)
8. [邮件配置 (SMTP)](#8-邮件配置-smtp)
9. [JWT 与鉴权](#9-jwt-与鉴权)
10. [多语言 (i18n)](#10-多语言-i18n)
11. [分页与展示配置](#11-分页与展示配置)
12. [编译与运行](#12-编译与运行)
13. [主站 API 列表](#13-主站-api-列表)
14. [管理后台 API 与功能](#14-管理后台-api-与功能)
15. [前端页面与路由](#15-前端页面与路由)
16. [常见问题与排查](#16-常见问题与排查)

---

## 1. 项目概述

- **项目名称**：EasyVideoWeb（原 boke，已统一更名）
- **技术栈**：Go 后端 + 静态前端（Vue 3 + 原生 JS）+ MySQL
- **主要功能**：视频与帖子发布、评论、点赞、收藏、消息、用户主页、审核、管理后台
- **模块**：
  - **主站**：`main.go` + `main_extension.go`，默认端口 **8080**
  - **管理后台**：`admin/admin_server.go`，默认端口 **8081**

---

## 2. 目录结构

```
E:\boke\（项目根目录）
├── main.go                 # 主站入口、路由、业务逻辑
├── main_extension.go       # 主站扩展逻辑（消息、审核等）
├── go.mod                  # Go 模块（module easyvideoweb）
├── app.config.json         # 应用配置：虚拟环境、分页
├── mysql.local.json        # MySQL 连接（需自行填写，勿提交敏感信息）
├── smtp.local.json         # SMTP 发信（需自行填写）
├── jwt.local.json          # JWT 密钥（由 jwt.local.example.json 复制后填写）
├── mysql.local.example.json # MySQL 配置示例（占位符）
├── smtp.local.example.json  # SMTP 配置示例（占位符）
├── jwt.local.example.json   # JWT 配置示例
├── storage/                 # 媒体与上传文件
│   ├── videos/
│   ├── avatars/
│   ├── banners/
│   └── posters/
├── uploads/                # 帖子等上传图片
├── web/                     # 前端静态资源
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
│   └── language/            # 多语言 JSON
│       ├── zh-CN.json       # 主站中文
│       ├── en-US.json       # 主站英文
│       ├── ru-RU.json       # 主站俄文
│       ├── backend-zh-CN.json  # 后端 API 中文
│       ├── backend-en-US.json
│       └── backend-ru-RU.json
├── admin/
│   └── admin_server.go     # 管理后台服务（独立可执行文件）
├── flash/                   # Python 虚拟环境（可选，见 app.config.json）
├── requirements_review.txt  # Python 审核依赖
└── DOCUMENTATION.md         # 本文档
```

---

## 3. 环境要求

| 项目       | 要求 |
|------------|------|
| Go         | 1.22+ |
| MySQL      | 5.7+ / 8.x，需提前建库或由程序建表 |
| FFmpeg     | 视频转码、缩略图与元数据解析需要，需加入系统 PATH |
| 浏览器     | 支持 ES6、Vue 3（CDN） |
| tiptop     | 审核相关依赖，需在虚拟环境中安装 |
| Python     | 仅当启用视频/内容审核时需要，见 [虚拟环境](#5-虚拟环境与-python-配置) |


---

## 4. 配置文件详解

### 4.1 总览

| 文件 | 用途 | 是否必须 | 是否提交到版本库 |
|------|------|----------|------------------|
| `app.config.json` | 虚拟环境开关、路径、分页 | 可选（有默认值） | 建议提交 |
| `mysql.local.json` | MySQL 连接信息 | **必须** | **不要**（用 example 占位） |
| `smtp.local.json` | SMTP 发信（注册/验证码等） | 发邮件功能需要 | **不要** |
| `jwt.local.json` | JWT 签名密钥 | **必须** | **不要** |
| `mysql.local.example.json` | MySQL 配置模板 | 参考用 | 可提交 |
| `smtp.local.example.json` | SMTP 配置模板 | 参考用 | 可提交 |
| `jwt.local.example.json` | JWT 配置模板 | 参考用 | 可提交 |

**环境变量可覆盖部分配置**（主站）：  
`MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASS` / `MYSQL_PASSWORD`, `MYSQL_DATABASE`。

---

### 4.2 app.config.json（应用配置）

路径：项目根目录 `app.config.json`。

```json
{
  "publish_quota": {
    "video_base": 3,
    "post_base": 3,
    "likes_per_bonus": 20,
    "comment": "每用户初始可发视频/帖子数；每获得 likes_per_bonus 赞额外 +1 次发布机会"
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
  }
}
```

- **publish_quota**（发布配额，主站启动时读取）  
  - **video_base**：每用户初始可发布视频数量，默认 3。  
  - **post_base**：每用户初始可发布帖子数量，默认 3。  
  - **likes_per_bonus**：每获得多少赞额外增加 1 次发布机会（视频与帖子共用规则），默认 20。  
  - 未提供 `app.config.json` 或未配置 `publish_quota` 时，使用代码内默认值（3/3/20）。

- **venv.use_venv**  
  - `true`：使用虚拟环境中的 Python（审核等脚本）。  
  - `false`：使用系统 `python`，不检查/安装 venv 依赖。

- **venv.venv_folder**  
  虚拟环境根目录名（相对项目根），如 `flash`。

- **venv.python_subpath** / **venv.pip_subpath**  
  相对 `venv_folder` 的路径：  
  - Windows：`Scripts/python.exe`、`Scripts/pip.exe`  
  - Linux/macOS：`bin/python`、`bin/pip`（需自行改配置）

- **pagination.videos_per_page**  
  首页视频列表每页条数配置项（当前前端未读取此值）。

- **pagination.posts_per_page**  
  帖子列表每页条数配置项（当前前端未读取此值）。

**未提供或字段缺失时**：  
- venv 默认 `use_venv: true`，`venv_folder: "flash"`，Windows 路径。  
- 分页当前由前端脚本默认值控制。

---

## 5. 虚拟环境与 Python 配置

### 5.1 何时需要 Python

- 视频/文本/图片等**审核逻辑**若通过 Python 脚本实现（如 `review_video.py`、`review_text.py`、`review_image.py`），则需配置 Python 环境。
- 若不需要审核或审核在别处实现，可在 `app.config.json` 中设 `use_venv: false`，主站不会依赖虚拟环境。

### 5.2 使用虚拟环境（推荐）

1. 在项目根目录创建虚拟环境，例如：
   ```bash
   python -m venv flash
   ```
2. 在 `app.config.json` 中设置：
   - `use_venv: true`
   - `venv_folder: "flash"`
   - `python_subpath`: Windows 为 `Scripts/python.exe`，Linux/Mac 为 `bin/python`
   - `pip_subpath`: Windows 为 `Scripts/pip.exe`，Linux/Mac 为 `bin/pip`
3. 主站启动时会：
   - 检查 `venv_folder` + `python_subpath` 是否存在；
   - 若存在且存在 `requirements_review.txt`，会尝试用该 venv 的 pip 安装依赖；
   - 审核时使用该 Python 解释器调用脚本。

### 5.3 使用系统 Python

- 在 `app.config.json` 中设 `"use_venv": false`。
- 主站将使用系统 PATH 中的 `python`，不再检查或安装 venv 依赖。

### 5.4 Linux / macOS 示例

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

## 6. AI 全流程审核

本项目的**视频、帖子、评论**在发布后均会进入 AI 审核流水线：由 Go 主站调用 Python 脚本，使用深度学习模型进行违规检测；通过则展示，不通过则拒绝或删除，并可选人工复审。

### 6.1 总体流程概览

| 内容类型 | 触发时机 | 审核方式 | 通过后 | 不通过后 | 是否支持人工复审 |
|----------|----------|----------|--------|----------|------------------|
| **视频** | 用户上传视频并写入库后 | 异步队列，调用 `review_video.py` | 状态改为 `approved`，首页可见 | 状态为 `rejected_violence` / `rejected_nsfw`，作者可见、可申请复审 | 是 |
| **帖子** | 用户发布帖子后 | 同步依次：文本 → 封面/正文图片 | 状态 `approved` | `rejected_abuse` / `rejected_nsfw` / `rejected_violence`，作者可申请复审 | 是 |
| **评论**（视频/帖子下） | 用户提交评论后 | 仅文本，调用 `review_text.py` | 状态 `approved` | **直接删除评论**并通知作者，无复审 | 否 |

- **视频审核**：最多 2 路并发（信号量控制），超时 10 分钟；脚本异常或超时时**默认通过**并写库，避免阻塞发布。
- **帖子审核**：文本超时 2 分钟、单张图片 3 分钟；任一步不通过即终止并写拒绝结果。
- **评论审核**：仅文本，超时 2 分钟；不通过则删除评论并写入 `comment_review_messages` 通知用户。

### 6.2 视频审核（review_video.py）

- **入口**：用户通过「视频投稿」上传成功后，Go 将 `video_library.review_status` 置为 `pending`，并调用 `enqueueVideoReview(videoID, filePath, authorEmail, title, description)`。队列最多 2 个任务同时执行。
- **流程**：先对视频文案（标题 + 描述）做文本审核（与帖子相同，使用 `review_text.py`），不通过则置为 `rejected_abuse` 并通知；通过后再执行画面审核。
- **脚本**：项目根目录 `review_video.py`，用法 `python review_video.py <视频文件路径>`。  
- **输出**：仅向 **stdout** 输出一行 JSON，如 `{"approved": true}` 或 `{"approved": false, "reject_reason": "violence"}`；进度与日志写 stderr。
- **流程**：  
  1. **抽帧**：OpenCV 读视频，按约 2 fps 抽帧（可配）。  
  2. **暴力检测**：  
     - 模型：VGG19 + LSTM，30 帧为一段，输入 160×160；权重文件：`Real-Time-Violence-Detection-in-Video-*/mamonbest947oscombo-drive.hdfs`（需自行放置或训练）。  
     - 任一段概率 ≥ 0.7 判定为暴力，立即返回 `reject_reason: "violence"`。  
  3. **裸露检测（NSFW）**：  
     - 模型：Caffe ResNet-50，`open_nsfw/nsfw_model/` 下 `deploy.prototxt` + `resnet_50_1by2_nsfw.caffemodel`。  
     - 逐帧 224×224 推理，平均 NSFW 概率 ≥ 0.7 或连续 3 帧及以上 ≥ 0.7 则判定不通过，返回 `reject_reason: "nsfw"`。  
- **结果写入**：Go 解析 stdout JSON，调用 `setReviewResult(videoID, status, ...)` 更新 `video_library.review_status`（`approved` / `rejected_abuse`（文案不通过）/ `rejected_violence` / `rejected_nsfw`），并向 `video_review_messages` 插入一条通知，作者在「消息中心」可见。
- **依赖**：Python 3、OpenCV、TensorFlow/Keras（暴力）、Caffe/OpenCV DNN（NSFW）。若模型文件缺失，对应检测会跳过并视为通过。

### 6.3 帖子审核（文本 + 图片）

- **入口**：用户发布帖子成功后，Go 在 goroutine 中调用 `enqueuePostReview(postID, contentText, contentHTML, imagePath, authorEmail, authorNickname, title)`，**同步**执行，不排队。
- **步骤**：  
  1. **文本审核**：将正文纯文本写入临时文件，执行 `review_text.py <临时文件>`。  
  2. **图片审核**：从帖子封面 `imagePath` 及正文 HTML 中解析出的 `/uploads/` 图片路径，逐张调用 `review_image.py <图片绝对路径>`。  
- **review_text.py**：  
  - 模型：ModelScope 文本分类 `damo/nlp_structbert_abuse-detect_chinese-tiny`（无风险 / 辱骂风险）。  
  - 长文按 300 字分段，任一段为「辱骂风险」则返回 `passed: false, reject_reason: "abuse"`。  
  - 若未安装 modelscope 或运行异常，脚本返回通过，避免阻塞发布。  
- **review_image.py**：  
  - 需放在项目根目录，与 `review_video.py` 同目录；输出 JSON 含 `approved`、`reject_reason`（如 `nsfw`、`violence`）。  
  - 任一张不通过则帖子状态设为 `rejected_nsfw` 或 `rejected_violence`，并写入 `post_review_messages`。  
- **结果**：全部通过则 `setReviewResultPost(..., "approved", ...)`；否则根据首次不通过原因设置 `rejected_abuse` / `rejected_nsfw` / `rejected_violence`，并通知作者。

### 6.4 评论审核（仅文本，不通过即删）

- **入口**：用户提交视频评论或帖子评论后，Go 插入评论时 `review_status` 为 `pending`，并异步调用 `runCommentReview(kind, commentID, videoID|postID, content, authorEmail, targetTitle)`。
- **逻辑**：  
  - 仅调用 `review_text.py`（同上，辱骂风险模型）。  
  - **通过**：更新 `video_comments` / `post_comments` 的 `review_status` 为 `approved`。  
  - **不通过**：**直接删除该评论**（及关联点赞），并向 `comment_review_messages` 插入通知，告知用户其评论因可能涉及歧视辱骂被删除。  
- **与视频/帖子区别**：评论不提供「人工复审」入口，AI 不通过即删除，仅保留通知记录。

### 6.5 人工复审（视频与帖子）

- **用户侧**：作者在「用户主页」→ 视频/帖子列表中，对状态为「未通过」或「已下架」的内容可点击「复审」，提交人工复审申请；Go 写入 `manual_video_reviews` / `manual_post_reviews`，状态 `pending`。
- **管理后台**：管理员在「视频人工复审区」「帖子人工复审区」看到待处理列表，可「通过」或「拒绝」并填写审核员与备注；通过后 `video_library` / `posts` 的 `review_status` 改为 `approved`，拒绝则保持或标记为拒绝。  
- **观看链接**：管理员查看待审视频/帖子时，会带 `adminReviewToken`，主站校验 `manual_review_access_tokens` 表后允许在未公开展示状态下播放/查看。

### 6.6 审核相关数据表与消息

- **视频**：`video_library.review_status`；`video_review_messages`（每轮审核结果通知作者）。  
- **帖子**：`posts.review_status`；`post_review_messages`。  
- **评论**：`video_comments.review_status` / `post_comments.review_status`；`comment_review_messages`（仅在不通过删除时写入）。  
- **人工复审**：`manual_video_reviews`、`manual_review_access_tokens`；`manual_post_reviews`、`manual_post_review_access_tokens`。

### 6.7 依赖与运行环境小结

- **Python**：由 `app.config.json` 的 venv 配置或系统 `python` 决定；审核脚本需能导入 `cv2`、`tensorflow`/`tf_keras`（视频）、`modelscope`（文本），以及 Caffe 模型文件（视频 NSFW）。  
- **模型与权重**：  
  - 视频暴力：`Real-Time-Violence-Detection-in-Video-*/mamonbest947oscombo-drive.hdfs`  
  - 视频 NSFW：`open_nsfw/nsfw_model/deploy.prototxt`、`resnet_50_1by2_nsfw.caffemodel`  
  - 文本辱骂：ModelScope 自动下载 `damo/nlp_structbert_abuse-detect_chinese-tiny`  
- **脚本异常/超时**：视频审核默认通过并更新 DB；帖子和评论在脚本异常时文本默认通过，图片单张失败可跳过或按实现决定是否整体不通过。  
- 关闭 AI 审核：若不需要审核，可在 `app.config.json` 中设 `use_venv: false`，并确保上传与发布逻辑不依赖审核结果（或改为默认通过）；也可保留 venv 但移除或替换脚本为始终通过的占位实现。

---

## 7. 数据库配置 (MySQL)

### 7.1 配置文件

- **实际使用**：`mysql.local.json`（不要提交到版本库）。  
- **模板**：`mysql.local.example.json`（可提交，占位符）。

### 7.2 mysql.local.example.json 与占位符

```json
{
  "host": "127.0.0.1",
  "port": "3306",
  "user": "your_mysql_user",
  "pass": "your_mysql_password",
  "database": "easyvideoweb"
}
```

部署步骤建议：

1. 复制 `mysql.local.example.json` 为 `mysql.local.json`。
2. 将 `your_mysql_user`、`your_mysql_password` 改为真实账号密码。
3. 将 `database` 改为实际库名（默认 `easyvideoweb`）。
4. 主站与 admin 均会读取 `mysql.local.json`；若 `pass` 为空或仍为 `your_mysql_password`，主站会报错并退出，提示需在 `mysql.local.json` 中配置密码。

### 7.3 环境变量（主站）

以下环境变量可覆盖 `mysql.local.json` 中对应项：

- `MYSQL_HOST`
- `MYSQL_PORT`
- `MYSQL_USER`
- `MYSQL_PASS` 或 `MYSQL_PASSWORD`
- `MYSQL_DATABASE`

---

## 8. 邮件配置 (SMTP)

### 8.1 用途

- 注册验证码、登录验证码、更换邮箱验证码等。
- 未正确配置 SMTP 时，相关接口会返回“邮件服务未配置”等错误。

### 8.2 配置文件

- **实际使用**：`smtp.local.json`（不要提交）。  
- **模板**：`smtp.local.example.json`。

示例（占位符）：

```json
{
  "host": "smtp.example.com",
  "port": "587",
  "user": "your_smtp_user",
  "pass": "your_smtp_password"
}
```

若 `user` 或 `pass` 为占位符 `your_smtp_user` / `your_smtp_password`，程序会视为未配置，不会发信。

### 8.3 常见 SMTP 示例

- Gmail：host `smtp.gmail.com`，port 587，开启“应用专用密码”。  
- QQ 邮箱：host `smtp.qq.com`，port 465 或 587，需开启 SMTP 并使用授权码。  
- 自建：按实际 host/port 填写，并确保防火墙放行。

---

## 9. JWT 与鉴权

### 9.1 jwt.local.json

- 主站从 `jwt.local.json` 读取 `secret`，用于签发与校验登录态。
- 若文件不存在或 `secret` 为空，主站会生成临时密钥（重启后变化，不推荐生产使用）。

### 9.2 示例与安全

- 复制 `jwt.local.example.json` 为 `jwt.local.json`。  
- 将 `secret` 改为**足够长、随机的字符串**，并妥善保管，不要提交到版本库。  
- 示例中为：`"secret": "easyvideoweb_jwt_secret_change_me"`，生产务必更换。

---

## 10. 多语言 (i18n)

### 10.1 支持语言

- **主站界面**：简体中文（zh-CN）、英文（en-US）、俄文（ru-RU）。  
- **后端 API 错误/提示**：同上，通过 backend 语言文件与请求头决定返回哪种语言。

### 10.2 主站语言文件（前端）

路径：`web/language/`。

| 文件 | 说明 |
|------|------|
| `zh-CN.json` | 中文界面文案（common、user、index、player、messages、posts、post_detail、creator、post_create、register_page 等） |
| `en-US.json` | 英文界面文案，结构与 zh-CN 一致 |
| `ru-RU.json` | 俄文界面文案，结构与 zh-CN 一致 |

前端通过 `GET /language/{locale}.json` 加载，例如 `/language/zh-CN.json`。  
界面文案 key 为层级结构，如 `common.header.videoHome`、`user.settings.language`。

### 10.3 后端 API 语言文件

路径：`web/language/`。

| 文件 | 说明 |
|------|------|
| `backend-zh-CN.json` | 中文 API 错误/提示（key 为下划线形式，如 `failed_to_load_categories`） |
| `backend-en-US.json` | 英文 |
| `backend-ru-RU.json` | 俄文 |

主站与 admin 都会加载上述 backend 文件，并根据请求头或管理端语言选择返回对应语言的错误信息。

### 10.4 默认语言与优先级

- **主站**  
  - 优先使用用户选择并保存在 **localStorage** 的 `evw_locale`（在用户主页 → 设置 → 语言 中切换）。  
  - 若未设置，则请求 **`GET /api/locale`**，服务端根据 **Accept-Language** 返回建议的 `locale`（zh-CN / en-US / ru-RU），前端再写入 `evw_locale` 并加载对应语言包。  
  - 未实现基于 IP 的自动语言，目前仅依据浏览器语言。

- **管理后台**  
  - 语言选择保存在 **localStorage** 的 `evw_admin_locale`。  
  - 所有请求会带上请求头 **`X-Locale`**，后台按此返回对应语言的错误信息。

### 10.5 主站切换语言

- 用户登录后进入 **用户主页** → **设置** 标签。  
- 在「语言」下拉框中选择：简体中文 / English / Русский。  
- 选择后会写入 `evw_locale` 并刷新当前页语言包，无需重新登录。

### 10.6 管理后台切换语言

- 打开管理后台页面，在顶部「语言 / Language」下拉框中选择。  
- 选择后写入 `evw_admin_locale`，后续 API 请求会带 `X-Locale`，错误提示将使用对应语言。

### 10.7 后端如何决定返回语言

- 主站：`getLocaleFromRequest(r)` 先看请求头 `X-Locale`，若无则解析 `Accept-Language`，映射到 zh-CN / en-US / ru-RU。  
- Admin：仅根据请求头 `X-Locale`（由前端根据 `evw_admin_locale` 设置）。  
- 错误文案通过 `writeBackendError(w, r, key, code)` / `writeAdminError(w, r, key, code)` 从对应语言的 backend JSON 中按 key 取出后返回。

---

## 11. 分页与展示配置

### 11.1 配置来源

- **前端脚本**：`web/app.js` 中 `state.pageSize`（首页视频列表），`web/posts.js` 中 `state.postPageSize`（帖子列表）。
- **服务端**：当前不提供分页配置接口，`/api/videos`、`/api/posts` 返回全量结果，由前端切页展示。

### 11.2 行为说明

- 首页（index）：视频列表每页条数 = `state.pageSize`（默认 12）。  
- 帖子列表页（posts）：帖子每页条数 = `state.postPageSize`（默认 10）。  
- 修改分页条数需调整 `web/app.js` 与 `web/posts.js` 中的默认值并重新部署前端静态资源。

### 11.3 本月最热排行榜

- **视频**：首页右侧「本月最热视频」来自 `GET /api/rankings/videos`，只统计**当月 1 号 0 点之后**发布且已通过的视频，按点赞、评论等综合得分排序，取前 10。
- **帖子**：帖子列表页右侧「帖子排行榜」来自 `GET /api/rankings/posts`，规则同上（当月帖子，按得分排序，前 10）。
- 排行榜在**主站进程启动时**计算一次，之后**每 12 小时**自动重算一次，无需手动刷新。

---

## 12. 编译与运行

编译得到的是 **Go 写的 Web 服务可执行文件**，不包含视频或前端资源。视频文件仍在 `storage/`、`uploads/` 等目录，由程序在运行时读取并对外提供访问。

### 12.1 主站

**开发时直接运行（推荐）：**

```bash
# 进入项目根目录
cd /path/to/boke

# 直接运行，无需先编译（修改代码后重新执行即可）
go run .
```

默认监听 **http://localhost:8080**。
### 12.2 管理后台

**开发时直接运行：**

```bash
cd /path/to/boke/admin
go run .
```



管理后台默认监听 **http://localhost:8081**。  
它从**当前工作目录**读取 `mysql.local.json`：若在 `admin` 子目录下执行，请把项目根目录的 `mysql.local.json` 复制到 `admin/`，或在项目根目录执行 `go run ./admin` 以共用同一配置。

### 12.3 首次部署检查清单

1. 在项目根目录复制并填写 `mysql.local.json`（勿使用占位符密码）。  
2. 复制并填写 `jwt.local.json`（生产环境务必更换 secret）。  
3. 如需邮件功能：复制并填写 `smtp.local.json`。  
4. 如需 Python 审核：按 [虚拟环境](#5-虚拟环境与-python-配置) 配置 `app.config.json` 并准备好 venv。  
5. 安装 **FFmpeg** 并加入系统 PATH（视频转码与缩略图需要）。  
6. 确认 MySQL 已创建对应数据库（或由程序自动建表）。  
7. 先启动主站，再按需启动管理后台。

---

## 13. 主站 API 列表

以下为部分主要接口，便于对照文档与排查。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/categories` | 视频分类列表 |
| GET | `/api/post-categories` | 帖子分类列表 |
| GET | `/api/videos` | 视频列表（query: category, q） |
| GET | `/api/posts` | 帖子列表（query: category, q） |
| GET | `/api/rankings/videos` | 本月最热视频排行（服务端启动时及每 12 小时重算） |
| GET | `/api/rankings/posts` | 本月最热帖子排行（同上） |
| GET | `/api/videos/{id}` | 视频详情 |
| GET | `/api/posts/{id}` | 帖子详情 |
| GET | `/api/locale` | 根据 Accept-Language 返回建议 locale |
| POST | `/api/send-code` | 发送验证码（注册等） |
| POST | `/api/verify-code` | 验证验证码并注册 |
| POST | `/api/login` | 密码登录 |
| POST | `/api/login-code/send` | 发送登录验证码 |
| POST | `/api/login-code/verify` | 验证码登录 |
| GET | `/api/profile` | 当前用户资料（需鉴权） |
| GET | `/api/users/profile` | 指定用户公开资料（query: email） |
| POST | `/api/profile/nickname` | 修改昵称 |
| POST | `/api/profile/password` | 修改/设置密码 |
| POST | `/api/profile/avatar` | 上传头像 |
| POST | `/api/profile/banner` | 上传封面 |
| POST | `/api/profile/notice` | 更新公告 |
| POST | `/api/profile/motto` | 更新座右铭 |
| GET | `/api/profile/favorites` | 用户视频收藏 |
| GET | `/api/profile/post-favorites` | 用户帖子收藏 |
| POST | `/api/creator/upload` | 上传视频 |
| GET | `/api/creator/publish-quota` | 发布配额 |
| POST | `/api/change-email/send` | 发送换绑邮箱验证码 |
| POST | `/api/change-email/verify` | 验证并换绑邮箱 |
| GET | `/api/messages` | 消息列表 |
| POST | `/api/messages/read` | 标记已读 |
| GET | `/api/messages/unread-count` | 未读数量 |
| POST | `/api/videos/favorite` | 视频收藏/取消 |
| POST | `/api/videos/like` | 视频点赞/取消 |
| POST | `/api/videos/view` | 记录播放 |
| POST | `/api/posts/favorite` | 帖子收藏/取消 |
| POST | `/api/posts/like` | 帖子点赞 |
| POST | `/api/posts/view` | 帖子浏览 |
| GET/POST/DELETE | `/api/videos/{id}/comments` 等 | 评论、点赞、删除等（见 main.go） |
| GET | `/api/homepage-posters` | 首页轮播图 |
| GET | `/api/system-notifications` | 系统通知 |
| GET | `/api/user-punishment` | 当前用户封禁/禁言状态 |

鉴权接口请在请求头中携带：`Authorization: Bearer <token>`。  
部分接口会根据请求头 `X-Locale` 或 `Accept-Language` 返回对应语言的错误信息（见 [多语言](#10-多语言-i18n)）。

---

## 14. 管理后台 API 与功能

- 管理后台为独立服务，默认端口 **8081**，HTML 内嵌在 `admin_server.go` 中。  
- 顶部提供 **语言选择**（zh-CN / en-US / ru-RU），选择后通过 `X-Locale` 请求头使 API 错误信息与所选语言一致。

主要功能与接口类型（具体路径以代码为准）：

- 视频分类、帖子分类的增删改查。  
- 系统通知的创建与列表。  
- 视频/帖子人工复审队列与操作。  
- 举报复审处理。  
- 首页海报上传与管理。  
- 批量上传视频。  
- 视频/帖子下架（takedown）。  
- 用户封禁、禁言、解封（pardon）。  
- 搜索：视频、帖子、用户。

错误文案从 `web/language/backend-*.json` 读取，与主站共用同一套 key。

---

## 15. 前端页面与路由

- 主站为单页/多页混合，路由多为静态 HTML 文件 + 前端 JS 调 API。  
- 语言包通过 `/language/{locale}.json` 加载，locale 来自 `evw_locale` 或 `/api/locale`。

| 路径/文件 | 说明 |
|-----------|------|
| `/`、`index.html` | 首页，视频列表、分类、轮播、分页 |
| `/posts.html` | 帖子列表、分类、分页 |
| `/user.html` | 用户主页（本人或他人），含设置、语言切换 |
| `/player.html` | 视频播放页 |
| `/creator.html` | 视频投稿 |
| `/post_create.html` | 发帖 |
| `/post_detail.html` | 帖子详情 |
| `/messages.html` | 消息中心 |
| `/register.html` | 注册页 |

用户主页 **设置** 中的「语言」下拉会保存到 `evw_locale`，并影响后续所有页面的界面语言。

---

## 16. 常见问题与排查

### 16.1 主站启动报错：mysql pass missing or not configured

- 未配置 `mysql.local.json`，或 `pass` 仍为占位符 `your_mysql_password`。  
- 解决：复制 `mysql.local.example.json` 为 `mysql.local.json`，填写真实 MySQL 密码。

### 16.2 注册/验证码/换邮箱时提示“邮件服务未配置”

- 未配置 `smtp.local.json`，或 `user`/`pass` 仍为 `your_smtp_user`/`your_smtp_password`。  
- 解决：复制 `smtp.local.example.json` 为 `smtp.local.json`，填写真实 SMTP 账号与密码（或应用专用密码）。

### 16.3 视频上传或处理失败

- 可能是 **FFmpeg** 未安装或未加入 PATH。请安装 FFmpeg 并确保运行主站的终端/环境中能执行 `ffmpeg`（若代码中用到 `ffprobe` 也需可用）。  
- 查看主站日志中是否有 FFmpeg 相关报错。

### 16.4 首页/帖子页每页条数不对

- 检查 `web/app.js` 的 `state.pageSize` 与 `web/posts.js` 的 `state.postPageSize`。  
- 修改后需要重新部署前端静态资源（仅重启主站无效）。

### 16.5 语言不切换或错误语言

- 主站：确认 localStorage 中 `evw_locale` 是否为 zh-CN / en-US / ru-RU 之一；清除缓存或重新在用户设置中选择语言。  
- 管理后台：确认顶部语言下拉与 localStorage 中 `evw_admin_locale` 一致；请求是否带 `X-Locale`（由页面脚本自动添加）。  
- 检查 `web/language/` 下是否存在对应 `{locale}.json` 与 `backend-{locale}.json`。

### 16.6 Python 审核相关报错

- 若不需要审核：在 `app.config.json` 中设 `use_venv: false`。  
- 若需要：确认 `venv_folder`、`python_subpath`、`pip_subpath` 正确，虚拟环境已创建，且 `requirements_review.txt` 已在该 venv 中安装成功。

### 16.7 管理后台无法连接数据库

- 管理后台与主站共用同一 `mysql.local.json`（从各自工作目录或可执行文件所在目录相对路径读取）。  
- 确保运行 admin 时当前目录或配置路径能正确找到 `mysql.local.json`，且数据库可访问。

---

## 附录：品牌与命名

- 项目、界面与文档中已统一使用 **EasyVideoWeb**（原 boke）。  
- 数据库名默认 **easyvideoweb**；配置文件、示例文件中的占位符与注释均已按此命名。  
- 前端 CSS 类名仍保留部分历史命名（如 `boke-header`），仅展示文案与标题为 EasyVideoWeb。

---

*文档版本与项目当前实现一致，如有代码变更请同步更新本文档。*
