import os
os.environ["DEEPFACE_HOME"] = "." 
import base64
import cv2
import numpy as np
from fastapi import FastAPI, File, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from deepface import DeepFace
import uvicorn

app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# --- NUEVO: Cargar el detector de lentes clásico de OpenCV ---
# Este archivo XML ya viene instalado dentro de la librería cv2
glasses_cascade = cv2.CascadeClassifier(cv2.data.haarcascades + 'haarcascade_eye_tree_eyeglasses.xml')

@app.post("/api/detect")
async def detect_faces(file: UploadFile = File(...)):
    try:
        contents = await file.read()
        nparr = np.frombuffer(contents, np.uint8)
        img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)

        _, buffer = cv2.imencode('.jpg', img)
        img_str = base64.b64encode(buffer).decode('utf-8')
        img_data_url = f"data:image/jpeg;base64,{img_str}"

        results = DeepFace.analyze(img, actions=['age', 'gender', 'emotion'], enforce_detection=False)
        
        if isinstance(results, dict):
            results = [results]
            
        caras_detectadas = []
        for face in results:
            genero = "male" if face.get("dominant_gender", "") == "Man" else "female"
            
            # --- NUEVA LÓGICA: Detectar Lentes ---
            tiene_lentes = False
            # DeepFace nos dice en qué coordenadas exactas está la cara
            region = face.get("region", {})
            x, y, w, h = region.get('x', 0), region.get('y', 0), region.get('w', 0), region.get('h', 0)
            
            if w > 0 and h > 0:
                # Recortamos la cara de la foto original para no buscar en el fondo
                cara_roi = img[y:y+h, x:x+w]
                gray_roi = cv2.cvtColor(cara_roi, cv2.COLOR_BGR2GRAY)
                
                # Le pasamos el escáner de lentes de OpenCV
                lentes = glasses_cascade.detectMultiScale(gray_roi, scaleFactor=1.1, minNeighbors=2)
                if len(lentes) > 0:
                    tiene_lentes = True # ¡Encontró lentes!

            caras_detectadas.append({
                "age": face.get("age"),
                "gender": genero,
                "emotion": face.get("dominant_emotion"),
                "glasses": tiene_lentes # Guardamos True o False
            })
            
        return {
            "status": "success",
            "img_url": img_data_url,
            "faces_count": len(caras_detectadas), 
            "faces": caras_detectadas
        }
        
    except Exception as e:
        return {"status": "error", "message": str(e)}

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8001)