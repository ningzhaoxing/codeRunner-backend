FROM python:3.11-slim

RUN useradd -m -u 10000 runner && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
USER runner

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

CMD ["python3", "main.py"]