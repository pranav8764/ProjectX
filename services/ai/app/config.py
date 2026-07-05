import os
import logging
from google import genai
from groq import Groq
import spacy

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("plantbrain-ai")

# Load environment variables
DATABASE_URL = os.getenv("DATABASE_URL", "postgresql://plantbrain:plantbrain@postgres:5432/plantbrain")
GOOGLE_API_KEY = os.getenv("GOOGLE_API_KEY")
GROQ_API_KEY = os.getenv("GROQ_API_KEY")

# Initialize Generative AI client
gemini_client = genai.Client(api_key=GOOGLE_API_KEY) if GOOGLE_API_KEY else None
groq_client = Groq(api_key=GROQ_API_KEY) if GROQ_API_KEY else None

# Load spaCy model
try:
    nlp = spacy.load("en_core_web_sm")
except OSError:
    import subprocess
    logger.info("Downloading spaCy model 'en_core_web_sm'...")
    subprocess.run(["python", "-m", "spacy", "download", "en_core_web_sm"])
    nlp = spacy.load("en_core_web_sm")
