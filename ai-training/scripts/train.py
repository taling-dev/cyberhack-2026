from ultralytics import YOLO
from datetime import datetime

# Get current date for the run name
date = datetime.now().strftime("%Y%m%d")

# Load the YOLOv8 classification model
model = YOLO('weights/yolov8n-cls.pt')

# Path to the prepared dataset
dataset_path = 'data/yolo_dataset'

# Start training
results = model.train(
    data=dataset_path, 
    epochs=150,
    imgsz=320,
    batch=16,
    project="kki_yolo_train",
    name=f"yolov8n_results_kki_{date}"
)

print("Training completed successfully.")
