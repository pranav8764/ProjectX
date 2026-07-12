FROM python:3.12-slim

# System deps for OCR (tesseract) and PDF rasterization (poppler). Table extraction
# uses pdfplumber (pure Python), so no Java runtime is needed.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    tesseract-ocr \
    poppler-utils \
    libgl1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy requirements and install python packages. Generous timeout/retries so a slow
# or flaky PyPI connection retries individual downloads instead of failing the build.
COPY requirements.txt .
RUN pip install --no-cache-dir --timeout=180 --retries=10 -r requirements.txt

# Download spaCy model
RUN python -m spacy download en_core_web_sm

# Copy service code
COPY . .

ENV PORT=8000
EXPOSE 8000

CMD ["python", "-m", "app.main"]
