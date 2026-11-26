import os
import threading
from pathlib import Path

import cv2
import numpy as np
from fastapi import FastAPI, File, HTTPException, UploadFile

app = FastAPI()
detector = None
recognizer = None
model_lock = threading.Lock()
DETECTION_RETRY_MAX_SIDES = (960, 480)


@app.on_event("startup")
def load_models():
    global detector, recognizer
    detector = cv2.FaceDetectorYN.create(os.environ["YUNET_MODEL_PATH"], "", (320, 320))
    recognizer = cv2.FaceRecognizerSF.create(os.environ["SFACE_MODEL_PATH"], "")


@app.get("/health/ready")
def ready():
    return {"ready": detector is not None and recognizer is not None}


def detect_faces(image):
    height, width = image.shape[:2]
    longest_side = max(width, height)
    with model_lock:
        detector.setInputSize((width, height))
        _, faces = detector.detect(image)
        if faces is not None and len(faces) > 0:
            return faces

    for retry_side in DETECTION_RETRY_MAX_SIDES:
        if longest_side <= retry_side:
            continue
        scale = retry_side / longest_side
        resized_width = max(1, round(width * scale))
        resized_height = max(1, round(height * scale))
        resized = cv2.resize(image, (resized_width, resized_height), interpolation=cv2.INTER_AREA)
        with model_lock:
            detector.setInputSize((resized_width, resized_height))
            _, faces = detector.detect(resized)
        if faces is not None and len(faces) > 0:
            # Map the box and landmarks back to the original photo for SFace alignment.
            faces[:, :14:2] *= width / resized_width
            faces[:, 1:14:2] *= height / resized_height
            return faces
    return None


@app.post("/ai/faces/embed")
async def embed(images: list[UploadFile] = File(...)):
    if not 3 <= len(images) <= 5:
        raise HTTPException(400, "Upload 3–5 photos")
    results = []
    for photo in images:
        data = await photo.read(5 * 1024 * 1024 + 1)
        result = {"filename": Path(photo.filename or "photo").name}
        if len(data) > 5 * 1024 * 1024:
            result["error"] = "Photo exceeds 5 MiB"
        else:
            try:
                image = cv2.imdecode(np.frombuffer(data, dtype=np.uint8), cv2.IMREAD_COLOR) if data else None
            except cv2.error:
                image = None
            if image is None:
                result["error"] = "Unreadable image"
            else:
                faces = detect_faces(image)
                if faces is None or len(faces) == 0:
                    result["error"] = "No face detected in photo"
                elif len(faces) > 1:
                    result["error"] = "Multiple faces detected. Use a photo with one person."
                else:
                    try:
                        with model_lock:
                            aligned = recognizer.alignCrop(image, faces[0])
                            embedding = recognizer.feature(aligned).flatten()
                        if embedding.size != 128 or not np.isfinite(embedding).all():
                            raise ValueError("Invalid embedding")
                        result["embedding"] = embedding.tolist()
                    except (cv2.error, ValueError):
                        result["error"] = "Face could not be processed"
        results.append(result)
    return {"model_version": os.environ["FACE_MODEL_VERSION"], "results": results}
