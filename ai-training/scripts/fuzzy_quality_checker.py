import numpy as np
import skfuzzy as fuzz
from skfuzzy import control as ctrl
from ultralytics import YOLO
import os

def create_fuzzy_system():
    # Antecedents (Inputs)
    # 1. Healthy Confidence (from YOLO)
    healthy_conf = ctrl.Antecedent(np.arange(0, 1.01, 0.01), 'healthy_conf')
    # 2. Defect Confidence (from YOLO)
    defect_conf = ctrl.Antecedent(np.arange(0, 1.01, 0.01), 'defect_conf')

    # Consequent (Output)
    # Quality score from 0 to 100
    quality = ctrl.Consequent(np.arange(0, 101, 1), 'quality')

    # Membership Functions
    healthy_conf['low'] = fuzz.trimf(healthy_conf.universe, [0, 0, 0.5])
    healthy_conf['medium'] = fuzz.trimf(healthy_conf.universe, [0.3, 0.6, 0.8])
    healthy_conf['high'] = fuzz.trimf(healthy_conf.universe, [0.6, 1, 1])

    defect_conf['none'] = fuzz.trimf(defect_conf.universe, [0, 0, 0.3])
    defect_conf['minor'] = fuzz.trimf(defect_conf.universe, [0.2, 0.5, 0.7])
    defect_conf['major'] = fuzz.trimf(defect_conf.universe, [0.5, 1, 1])

    quality['reject'] = fuzz.trapmf(quality.universe, [0, 0, 20, 40])
    quality['standard'] = fuzz.trimf(quality.universe, [30, 60, 80])
    quality['premium'] = fuzz.trapmf(quality.universe, [70, 90, 100, 100])

    # Fuzzy Rules
    rule1 = ctrl.Rule(healthy_conf['high'] & defect_conf['none'], quality['premium'])
    rule2 = ctrl.Rule(healthy_conf['medium'] & defect_conf['minor'], quality['standard'])
    rule3 = ctrl.Rule(defect_conf['major'], quality['reject'])
    rule4 = ctrl.Rule(healthy_conf['low'], quality['reject'])

    # Control System
    quality_ctrl = ctrl.ControlSystem([rule1, rule2, rule3, rule4])
    return ctrl.ControlSystemSimulation(quality_ctrl)

def run_quality_check(model_path, image_path):
    if not os.path.exists(model_path):
        print(f"Error: Model not found at {model_path}. Training might still be in progress.")
        return

    # Load YOLO Model
    model = YOLO(model_path)
    
    # Run Inference
    results = model(image_path)[0]
    
    # Extract Confidence Scores
    # Note: We need to map class indices to 'Healthy' or 'Disease'
    # For simplicity, we assume 'Healthy' and 'Disease' are the top two classes
    probs = results.probs
    class_names = results.names
    
    # Mapping specific dataset classes to fuzzy inputs
    # Healthy: Apple___healthy, Citrus Healthy Fruit, Guava Healthly Fruit, Healthy Apple Fruit, Healthy Banana Fruit, Healthy Mango Fruit, Papaya Fruit Healthy
    # Defect: Apple___Black_rot, Banana cordana, Citrus Fruit disease, Disease Apple Fruit, Disease Banana Fruit, Disease Mango Fruit, Guava Disease Fruit, Guava Red Rust, Mango Bacterial Canker, Papaya Fruit Disease, Papaya RingSpot
    
    healthy_score = 0
    defect_score = 0
    
    # Class mapping based on your specific dataset folders
    healthy_keywords = ['healthy']
    defect_keywords = ['disease', 'rot', 'rust', 'canker', 'cordana', 'ringspot']
    
    for i, score in enumerate(probs.data.tolist()):
        name = class_names[i].lower()
        is_healthy = any(k in name for k in healthy_keywords)
        is_defect = any(k in name for k in defect_keywords)
        
        if is_healthy:
            healthy_score = max(healthy_score, score)
        elif is_defect:
            defect_score = max(defect_score, score)

    print(f"\n--- Analysis Results ---")
    print(f"Healthy Signal: {healthy_score:.2%}")
    print(f"Defect Signal:  {defect_score:.2%}")
    print(f"------------------------")

    # Run Fuzzy Logic
    sys = create_fuzzy_system()
    sys.input['healthy_conf'] = healthy_score
    sys.input['defect_conf'] = defect_score
    
    try:
        sys.compute()
        final_quality = sys.output['quality']
        
        # Grading with human-readable explanation
        if final_quality >= 85:
            grade = "PREMIUM (Grade A)"
            desc = "Excellent quality, fresh and healthy."
        elif final_quality >= 60:
            grade = "STANDARD (Grade B)"
            desc = "Good quality, minor imperfections detected."
        elif final_quality >= 40:
            grade = "LOW GRADE (Grade C)"
            desc = "Usable but significant defects present. Suggest discount."
        else:
            grade = "REJECT (Grade D)"
            desc = "Major disease or rot. Unfit for consumption."
            
        print(f"Quality Score: {final_quality:.1f}/100")
        print(f"Verdict:       {grade}")
        print(f"Details:       {desc}\n")
        
    except Exception as e:
        print(f"Fuzzy computation error: {e}. Check if inputs triggered any rules.")

if __name__ == "__main__":
    # Pointing to the finalized high-power weights
    MODEL_PATH = 'runs/classify/kki_yolo_train/yolov8s_high_power/weights/best.pt'
    IMAGE_PATH = 'test_fruit.jpg' # Replace with a real test image
    
    print("YOLO + Fuzzy Logic Quality Checker")
    run_quality_check(MODEL_PATH, IMAGE_PATH)
