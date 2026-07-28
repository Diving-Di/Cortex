FROM python:3.13.5-slim-bookworm

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && useradd --system --uid 10001 --home /nonexistent --shell /usr/sbin/nologin parser
COPY app.py .
USER parser
EXPOSE 8090
CMD ["python", "/app/app.py"]
