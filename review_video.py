#!/usr/bin/env python3
"""
Video Review Script - Violence & NSFW Detection
Usage: python review_video.py <video_path>
Output: JSON to stdout, progress logs to stderr
"""

import sys
import os

# 立即将 stdout 重定向到 stderr，防止任何第三方库的 print 污染 JSON 输出
_real_stdout = sys.stdout
sys.stdout = sys.stderr

# 强制使用 Keras 2 以兼容旧版 .hdfs/.hdf5 权重格式
os.environ['TF_USE_LEGACY_KERAS'] = '1'
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'

import json
import time
import traceback
import numpy as np
import cv2

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
VIOLENCE_WEIGHTS = os.path.join(SCRIPT_DIR, "Real-Time-Violence-Detection-in-Video-", "mamonbest947oscombo-drive.hdfs")
NSFW_PROTOTXT = os.path.join(SCRIPT_DIR, "open_nsfw", "nsfw_model", "deploy.prototxt")
NSFW_CAFFEMODEL = os.path.join(SCRIPT_DIR, "open_nsfw", "nsfw_model", "resnet_50_1by2_nsfw.caffemodel")

VIOLENCE_THRESHOLD = 0.7
NSFW_THRESHOLD = 0.7
VIOLENCE_SEGMENT_FRAMES = 30
FRAME_SIZE_VIOLENCE = 160
NSFW_FRAME_SIZE = 224

_violence_model = None


def log(msg):
    print(f"[review] {msg}", flush=True)


def write_json_result(obj):
    """将 JSON 结果写入真正的 stdout（不受重定向影响）"""
    _real_stdout.write(json.dumps(obj))
    _real_stdout.flush()


def extract_frames(video_path, fps_target=2):
    log(f"正在打开视频: {video_path}")
    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        raise RuntimeError(f"Cannot open video: {video_path}")

    video_fps = cap.get(cv2.CAP_PROP_FPS)
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    duration = total_frames / video_fps if video_fps > 0 else 0
    if video_fps <= 0:
        video_fps = 30.0

    log(f"视频信息: FPS={video_fps:.1f}, 总帧数={total_frames}, 时长={duration:.1f}s")

    frame_interval = max(1, int(video_fps / fps_target))
    log(f"抽帧间隔: 每{frame_interval}帧取1帧 (目标{fps_target}fps)")
    frames = []
    frame_idx = 0

    while True:
        ret, frame = cap.read()
        if not ret:
            break
        if frame_idx % frame_interval == 0:
            frames.append(frame)
        frame_idx += 1

    cap.release()
    log(f"抽帧完成: 共提取 {len(frames)} 帧")
    return frames


def load_violence_model():
    global _violence_model
    if _violence_model is not None:
        log("暴力检测模型已缓存，跳过加载")
        return _violence_model

    log("正在加载暴力检测模型 (VGG19 + LSTM)...")
    t0 = time.time()

    import tensorflow as tf
    tf.get_logger().setLevel('ERROR')
    from tf_keras.applications.vgg19 import VGG19
    from tf_keras import layers, models

    base_model = VGG19(include_top=False, weights='imagenet', input_shape=(160, 160, 3))
    cnn = models.Sequential([base_model, layers.Flatten()])

    model = models.Sequential([
        layers.TimeDistributed(cnn, input_shape=(30, 160, 160, 3)),
        layers.LSTM(30, return_sequences=True),
        layers.TimeDistributed(layers.Dense(90)),
        layers.Dropout(0.1),
        layers.GlobalAveragePooling1D(),
        layers.Dense(512, activation='relu'),
        layers.Dropout(0.3),
        layers.Dense(2, activation='sigmoid'),
    ])

    log(f"正在加载权重: {VIOLENCE_WEIGHTS}")
    model.load_weights(VIOLENCE_WEIGHTS)
    model.compile(loss='binary_crossentropy', optimizer='adam', metrics=['accuracy'])
    _violence_model = model
    log(f"暴力检测模型加载完成 (耗时 {time.time()-t0:.1f}s)")
    return model


def check_violence(frames):
    log("=" * 50)
    log("开始暴力检测")
    log("=" * 50)

    if not os.path.exists(VIOLENCE_WEIGHTS):
        log(f"警告: 暴力检测模型未找到: {VIOLENCE_WEIGHTS}")
        log("跳过暴力检测")
        return {"passed": True, "skipped": True, "reason": "model_not_found", "confidence": 0}

    try:
        model = load_violence_model()
    except Exception as e:
        log(f"错误: 暴力检测模型加载失败: {e}")
        return {"passed": True, "skipped": True, "reason": f"model_load_error: {e}", "confidence": 0}

    segments = []
    for i in range(0, len(frames), VIOLENCE_SEGMENT_FRAMES):
        seg = frames[i:i + VIOLENCE_SEGMENT_FRAMES]
        if len(seg) < VIOLENCE_SEGMENT_FRAMES:
            while len(seg) < VIOLENCE_SEGMENT_FRAMES:
                seg.append(seg[-1])
        segments.append(seg)

    if not segments:
        log("警告: 无可用帧片段")
        return {"passed": True, "reason": "no_frames", "confidence": 0}

    log(f"分段完成: {len(segments)} 个片段, 每段 {VIOLENCE_SEGMENT_FRAMES} 帧")

    max_violence = 0.0
    for seg_idx, seg in enumerate(segments):
        log(f"  推理片段 [{seg_idx+1}/{len(segments)}]...")
        t0 = time.time()
        data = np.zeros((1, 30, 160, 160, 3), dtype=np.float32)
        for idx, frame in enumerate(seg):
            resized = cv2.resize(frame, (FRAME_SIZE_VIOLENCE, FRAME_SIZE_VIOLENCE))
            resized = cv2.cvtColor(resized, cv2.COLOR_BGR2RGB).astype(np.float32) / 255.0
            data[0][idx] = resized

        pred = model.predict(data, verbose=0)
        prob = float(pred[0][1])
        elapsed = time.time() - t0
        max_violence = max(max_violence, prob)

        status = "暴力" if prob >= VIOLENCE_THRESHOLD else "正常"
        log(f"  片段 [{seg_idx+1}/{len(segments)}] 暴力概率={prob:.4f} ({status}) 耗时={elapsed:.1f}s")

        if prob >= VIOLENCE_THRESHOLD:
            log(f"检测到暴力内容! 概率={prob:.4f} >= 阈值{VIOLENCE_THRESHOLD}")
            return {
                "passed": False,
                "reason": "violence_detected",
                "confidence": round(prob, 4),
            }

    log(f"暴力检测通过 (最高概率={max_violence:.4f}, 阈值={VIOLENCE_THRESHOLD})")
    return {"passed": True, "reason": "", "confidence": round(max_violence, 4)}


def check_nsfw(frames):
    log("=" * 50)
    log("开始裸露检测")
    log("=" * 50)

    if not os.path.exists(NSFW_PROTOTXT) or not os.path.exists(NSFW_CAFFEMODEL):
        log(f"警告: NSFW模型未找到")
        log(f"  prototxt: {NSFW_PROTOTXT} (存在={os.path.exists(NSFW_PROTOTXT)})")
        log(f"  caffemodel: {NSFW_CAFFEMODEL} (存在={os.path.exists(NSFW_CAFFEMODEL)})")
        log("跳过裸露检测")
        return {"passed": True, "skipped": True, "reason": "model_not_found", "confidence": 0}

    try:
        log("正在加载NSFW模型 (ResNet-50 Caffe)...")
        t0 = time.time()
        net = cv2.dnn.readNetFromCaffe(NSFW_PROTOTXT, NSFW_CAFFEMODEL)
        log(f"NSFW模型加载完成 (耗时 {time.time()-t0:.1f}s)")
    except Exception as e:
        log(f"错误: NSFW模型加载失败: {e}")
        return {"passed": True, "skipped": True, "reason": f"model_load_error: {e}", "confidence": 0}

    nsfw_probs = []
    consecutive_nsfw = 0
    max_consecutive = 0

    log(f"开始逐帧推理 ({len(frames)} 帧)...")
    t0 = time.time()
    for i, frame in enumerate(frames):
        resized = cv2.resize(frame, (NSFW_FRAME_SIZE, NSFW_FRAME_SIZE))
        blob = cv2.dnn.blobFromImage(resized, 1.0, (NSFW_FRAME_SIZE, NSFW_FRAME_SIZE),
                                     (104, 117, 123), swapRB=False)
        net.setInput(blob)
        output = net.forward()
        prob = float(output[0][1])
        nsfw_probs.append(prob)

        if prob >= NSFW_THRESHOLD:
            consecutive_nsfw += 1
            max_consecutive = max(max_consecutive, consecutive_nsfw)
        else:
            consecutive_nsfw = 0

        if (i + 1) % 10 == 0 or i == len(frames) - 1:
            log(f"  已处理 [{i+1}/{len(frames)}] 帧, 当前帧NSFW={prob:.4f}, 连续NSFW帧={consecutive_nsfw}")

    elapsed = time.time() - t0
    if not nsfw_probs:
        log("警告: 无NSFW推理结果")
        return {"passed": True, "reason": "no_frames", "confidence": 0}

    avg_nsfw = sum(nsfw_probs) / len(nsfw_probs)
    max_nsfw = max(nsfw_probs)

    log(f"裸露检测推理完成 (耗时 {elapsed:.1f}s)")
    log(f"  平均NSFW概率={avg_nsfw:.4f}")
    log(f"  最高NSFW概率={max_nsfw:.4f}")
    log(f"  最大连续NSFW帧数={max_consecutive}")

    if avg_nsfw >= NSFW_THRESHOLD or max_consecutive >= 3:
        log(f"检测到裸露内容! 平均={avg_nsfw:.4f} 连续={max_consecutive}")
        return {
            "passed": False,
            "reason": "nsfw_detected",
            "confidence": round(max(avg_nsfw, max_nsfw), 4),
        }

    log(f"裸露检测通过 (平均={avg_nsfw:.4f}, 阈值={NSFW_THRESHOLD})")
    return {"passed": True, "reason": "", "confidence": round(avg_nsfw, 4)}


def main():
    if len(sys.argv) < 2:
        write_json_result({"error": "usage: review_video.py <video_path>"})
        sys.exit(1)

    video_path = sys.argv[1]
    if not os.path.exists(video_path):
        log(f"错误: 视频文件不存在: {video_path}")
        write_json_result({"error": f"video not found: {video_path}"})
        sys.exit(1)

    log("=" * 50)
    log(f"开始审核视频: {os.path.basename(video_path)}")
    log("=" * 50)
    review_start = time.time()

    result = {
        "video_path": video_path,
        "approved": False,
        "reject_reason": "",
        "violence": None,
        "nsfw": None,
    }

    try:
        frames = extract_frames(video_path, fps_target=2)
        if not frames:
            log("错误: 未能提取任何帧")
            result["error"] = "no_frames_extracted"
            write_json_result(result)
            return

        violence_result = check_violence(frames)
        result["violence"] = violence_result

        if not violence_result["passed"]:
            result["approved"] = False
            result["reject_reason"] = "violence"
            log("=" * 50)
            log("审核结果: 未通过 (暴力内容)")
            log(f"总耗时: {time.time()-review_start:.1f}s")
            log("=" * 50)
            write_json_result(result)
            return

        nsfw_result = check_nsfw(frames)
        result["nsfw"] = nsfw_result

        if not nsfw_result["passed"]:
            result["approved"] = False
            result["reject_reason"] = "nsfw"
            log("=" * 50)
            log("审核结果: 未通过 (裸露内容)")
            log(f"总耗时: {time.time()-review_start:.1f}s")
            log("=" * 50)
            write_json_result(result)
            return

        result["approved"] = True
        log("=" * 50)
        log("审核结果: 通过")
        log(f"总耗时: {time.time()-review_start:.1f}s")
        log("=" * 50)
    except Exception:
        tb = traceback.format_exc()
        log(f"审核异常:\n{tb}")
        result["error"] = tb
        result["approved"] = False
        result["reject_reason"] = "error"

    write_json_result(result)


if __name__ == "__main__":
    main()
