FROM python:3.12-slim

WORKDIR /app

# System libs required by opencv/ultralytics (libGL, glib) for image decode.
RUN apt-get update && apt-get install -y --no-install-recommends \
      libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Copy the full source first so the simaops_proto package exists when the
# build backend assembles the wheel, then install base + inference deps
# (ultralytics, scikit-fuzzy, minio, opencv) into the image.
COPY . .
RUN pip install --no-cache-dir ".[inference]"

# Pre-bundled trained model lives at models/best.pt (see MODEL_PATH).
ENV MODEL_PATH=models/best.pt

EXPOSE 8081
CMD ["python", "main.py"]
