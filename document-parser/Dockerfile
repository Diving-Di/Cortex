FROM python:3.12-slim-bookworm

RUN apt-get update && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends \
    antiword=0.37-16 tesseract-ocr=5.3.0-2 tesseract-ocr-chi-sim=1:4.1.0-2 \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
RUN useradd --system --uid 10001 --no-create-home parser
USER parser
EXPOSE 8080
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080", "--workers", "1", "--limit-concurrency", "4"]
