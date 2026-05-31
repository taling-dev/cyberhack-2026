# Indian Fruit Crop Disease Classification & Quality Grading

This project uses YOLOv8 and Fuzzy Logic to classify diseases in Indian fruit crops and provide a quality grade based on the detection confidence.

## 🚀 Overview

The system consists of two main components:
1. **YOLOv8 Classifier:** Trained to identify various fruit diseases and healthy states across multiple fruit types (Apple, Banana, Citrus, Guava, Mango, Papaya).
2. **Fuzzy Logic Grader:** A rule-based system that takes the confidence scores from the YOLOv8 model to calculate a quality score (0-100) and assign a human-readable grade (Premium, Standard, Low Grade, or Reject).

## 📊 Dataset

The project uses the **"6 Indian Fruit crops Fruit and Leaf Disease Dataset"**. 
Note: The preprocessing script is configured to filter out leaf-related classes, focusing specifically on **fruit disease detection**.

## 🛠️ Installation

Ensure you have Python 3.8+ installed. Install the required dependencies:

```bash
pip install ultralytics scikit-fuzzy numpy
```

## 📂 Project Structure

- `data/`: Contains raw and prepared datasets.
- `weights/`: Pre-trained YOLOv8 model weights.
- `scripts/`: Python scripts for training, data splitting, and quality checking.
- `runs/`: Directory containing training logs and model weights.

## 🏃 Usage

### 1. Prepare the Dataset
Run the split script to create the `data/yolo_dataset` directory:
```bash
python scripts/split_dataset.py
```

### 2. Train the Model
Train the YOLOv8n-cls model:
```bash
python scripts/train.py
```

### 3. Run Quality Check
Perform inference on a test image:
```bash
python scripts/fuzzy_quality_checker.py
```
*Note: You may need to update the `MODEL_PATH` and `IMAGE_PATH` variables in the script to point to your specific model weights and test image.*

## 🧠 Fuzzy Logic System

The grading system uses the following inputs:
- **Healthy Confidence:** The model's confidence that the fruit is healthy.
- **Defect Confidence:** The highest confidence score among detected disease classes.

**Grades:**
- **PREMIUM (Grade A):** High healthy confidence, no defects.
- **STANDARD (Grade B):** Medium healthy confidence, minor defects.
- **LOW GRADE (Grade C):** Significant defects present.
- **REJECT (Grade D):** Major disease or rot detected.

## 📈 Results
Training results, including confusion matrices and accuracy plots, can be found in the `runs/classify/` directory after a successful training run.

## 🔌 System Integration

This system is designed to be modular and can be integrated into various agricultural and industrial workflows:

### 1. Web & Mobile Applications (REST API)
You can wrap the logic in `fuzzy_quality_checker.py` using **FastAPI** or **Flask** to create an API. This allows mobile apps for farmers or quality inspectors to upload images and receive instant grading results in JSON format.

### 2. Automated Sorting Systems (Edge Computing)
For real-time sorting on conveyor belts, the model can be deployed on edge devices like **NVIDIA Jetson** or **Raspberry Pi**. The quality grade can trigger mechanical actuators (via GPIO) to sort fruit into different bins (e.g., Export, Local Market, Processed, or Waste).

### 3. Batch Quality Auditing
Integrate the scripts into a cloud pipeline (e.g., AWS Lambda or Google Cloud Functions) to automatically process batches of fruit images uploaded to storage buckets, generating daily quality reports for large-scale warehouses.

### 4. Continuous Improvement
By logging instances where the model's verdict is manually corrected by a human expert, you can build a feedback loop to periodically retrain the YOLO model using `train.py`, ensuring the system adapts to new fruit varieties or seasonal variations.
