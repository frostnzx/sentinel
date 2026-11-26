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


@app.on_event("startup")
def load_models():
    global detector, recognizer
    detector = cv2.FaceDetectorYN.create(os.environ["YUNET_MODEL_PATH"], "", (320, 320))
    recognizer = cv2.FaceRecognizerSF.create(os.environ["SFACE_MODEL_PATH"], "")


@app.get("/health/ready")
def ready():
    return {"ready": detector is not None and recognizer is not None}


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
                with model_lock:
                    detector.setInputSize((image.shape[1], image.shape[0]))
                    _, faces = detector.detect(image)
                if faces is None or len(faces) != 1:
                    result["error"] = "Photo must contain exactly one detectable face"
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
