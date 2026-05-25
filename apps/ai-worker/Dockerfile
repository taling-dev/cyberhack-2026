FROM python:3.12-slim

WORKDIR /app
COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY . .

EXPOSE 8081
CMD ["python", "main.py"]
