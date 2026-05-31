import ultralytics 
import os
from ultralytics import YOLO

def train_model(data_path, model_name, epochs, batch_size, image_size, patience=50):
    # Load the YOLO model
    model = YOLO(model_name)

    # Train the model
    model.train(data=data_path, epochs=epochs, batch=batch_size, imgsz=image_size, patience=patience)

if __name__ == "__main__":
    # Define the path to your dataset and model parameters
    data_path = "/home/mamat/CYBERHACK-2026/datasets/fruits_detect/data.yaml"  
    model_name = "yolov8s.pt"  
    epochs = 300  
    batch_size = 8
    image_size = 640
    patience = 20

    # Train the model
    train_model(data_path, model_name, epochs, batch_size, image_size, patience)