import os
import shutil
import random

def split_dataset(source_dir, output_dir, split_ratio=0.8):
    if not os.path.exists(output_dir):
        os.makedirs(output_dir)
    
    train_dir = os.path.join(output_dir, 'train')
    val_dir = os.path.join(output_dir, 'val')
    
    os.makedirs(train_dir, exist_ok=True)
    os.makedirs(val_dir, exist_ok=True)
    
    classes = [d for d in os.listdir(source_dir) if os.path.isdir(os.path.join(source_dir, d))]
    
    # Filter out classes that contain 'Leaf' or 'leaf'
    classes = [cls for cls in classes if 'Leaf' not in cls and 'leaf' not in cls]
    
    for cls in classes:
        print(f"Processing class: {cls}")
        cls_source_dir = os.path.join(source_dir, cls)
        cls_train_dir = os.path.join(train_dir, cls)
        cls_val_dir = os.path.join(val_dir, cls)
        
        os.makedirs(cls_train_dir, exist_ok=True)
        os.makedirs(cls_val_dir, exist_ok=True)
        
        images = [f for f in os.listdir(cls_source_dir) if os.path.isfile(os.path.join(cls_source_dir, f))]
        random.shuffle(images)
        
        split_point = int(len(images) * split_ratio)
        train_images = images[:split_point]
        val_images = images[split_point:]
        
        for img in train_images:
            shutil.copy(os.path.join(cls_source_dir, img), os.path.join(cls_train_dir, img))
        
        for img in val_images:
            shutil.copy(os.path.join(cls_source_dir, img), os.path.join(cls_val_dir, img))

if __name__ == "__main__":
    src = "data/6  indian Fruit crops Fruit and Leaf Disease Dataset/6  indian Fruit crops Fruit and Leaf Disease Dataset"
    dst = "data/yolo_dataset"
    split_dataset(src, dst)
    print("Dataset split complete.")
