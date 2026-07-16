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

# Model IDs (env-overridable; defaults must be currently-served models — the previous
# hardcoded gemini-1.5-pro / llama3-70b-8192 are retired and 404 on fresh API keys)
GEMINI_MODEL = os.getenv("GEMINI_MODEL", "gemini-2.0-flash")
GEMINI_EMBED_MODEL = os.getenv("GEMINI_EMBED_MODEL", "gemini-embedding-001")
# Must match the pgvector column dimension (vector(1536)). gemini-embedding-001 emits
# 1536 natively via output_dimensionality; only 3072 is pre-normalized, so we L2-normalize.
GEMINI_EMBED_DIM = int(os.getenv("GEMINI_EMBED_DIM", "1536"))
GROQ_MODEL = os.getenv("GROQ_MODEL", "llama-3.3-70b-versatile")

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
