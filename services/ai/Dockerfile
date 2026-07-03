FROM python:3.10-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    tesseract-ocr \
    poppler-utils \
    libgl1-mesa-glx \
    default-jdk \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy requirements and install python packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Download spaCy model
RUN python -m spacy download en_core_web_sm

# Copy service code
COPY . .

ENV PORT=8000
EXPOSE 8000

CMD ["python", "app/main.py"]
