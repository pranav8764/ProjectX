import os
import asyncio
import logging
import uuid
import json
import re
from datetime import datetime
from typing import Dict, Any, Optional, List

import asyncpg
import numpy as np
from fastapi import FastAPI, HTTPException, BackgroundTasks
from pydantic import BaseModel

import PyPDF2
import pdfplumber
from docx import Document as DocxDocument
import pandas as pd
from PIL import Image
import pytesseract
import pdf2image
import spacy
import tabula

# Generative AI APIs
import google.generativeai as genai
from groq import Groq

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("plantbrain-ai")

# Initialize FastAPI app
app = FastAPI(title="PlantBrainAI Consolidated AI Service", version="1.0.0")

# Load environment variables
DATABASE_URL = os.getenv("DATABASE_URL", "postgresql://plantbrain:plantbrain@postgres:5432/plantbrain")
GOOGLE_API_KEY = os.getenv("GOOGLE_API_KEY")
GROQ_API_KEY = os.getenv("GROQ_API_KEY")

# Initialize Generative AI client
if GOOGLE_API_KEY:
    genai.configure(api_key=GOOGLE_API_KEY)
groq_client = Groq(api_key=GROQ_API_KEY) if GROQ_API_KEY else None

# Load spaCy model
try:
    nlp = spacy.load("en_core_web_sm")
except OSError:
    import subprocess
    subprocess.run(["python", "-m", "spacy", "download", "en_core_web_sm"])
    nlp = spacy.load("en_core_web_sm")

# Database connection pool
db_pool: Optional[asyncpg.Pool] = None

@app.on_event("startup")
async def startup_event():
    global db_pool
    for i in range(5):
        try:
            db_pool = await asyncpg.create_pool(DATABASE_URL)
            logger.info("Successfully connected to PostgreSQL")
            break
        except Exception as e:
            logger.warning(f"Database connection failed, retrying in 2s ({i+1}/5): {e}")
            await asyncio.sleep(2)
    if not db_pool:
        logger.error("Failed to connect to database. AI Service starting without DB pool.")

@app.on_event("shutdown")
async def shutdown_event():
    global db_pool
    if db_pool:
        await db_pool.close()
    logger.info("Database connection closed")

@app.get("/health")
async def health_check():
    return {"status": "healthy", "service": "ai-service"}

# Pydantic Schemas
class DocumentProcessRequest(BaseModel):
    document_id: str
    file_path: str
    file_type: str
    metadata: Dict[str, Any]

class CopilotQueryRequest(BaseModel):
    question: str
    plantId: str
    filters: Optional[Dict[str, Any]] = None

class RCAGenerateRequest(BaseModel):
    assetTag: str
    failureDescription: str

# Helper: Gemini 1536-dim Embedding Generator
async def get_gemini_embedding_1536(text: str) -> List[float]:
    """
    Generates 1536-dimensional embeddings using Gemini text-embedding-004.
    Since text-embedding-004 produces 768 dimensions by default, we concatenate the vector 
    with itself to yield exactly 1536 dimensions. This preserves cosine similarity mathematically.
    """
    if not GOOGLE_API_KEY:
        # Return fallback zero embedding if no key is provided
        return [0.0] * 1536
    
    try:
        # Run synchronous call in thread pool to avoid blocking async loop
        loop = asyncio.get_event_loop()
        response = await loop.run_in_executor(
            None,
            lambda: genai.embed_content(
                model="models/text-embedding-004",
                content=text,
                task_type="retrieval_document"
            )
        )
        emb_768 = response['embedding']
        # Concatenate vector with itself to reach 1536 dimensions
        emb_1536 = emb_768 + emb_768
        return emb_1536
    except Exception as e:
        logger.error(f"Error generating Gemini embedding: {e}")
        # Return fallback zero embedding
        return [0.0] * 1536

# Helper: LLM Generator
async def generate_text_llm(prompt: str) -> str:
    """Helper to generate text using Gemini-1.5-pro or Groq Llama3"""
    if GOOGLE_API_KEY:
        try:
            loop = asyncio.get_event_loop()
            model = genai.GenerativeModel('gemini-1.5-pro')
            response = await loop.run_in_executor(None, lambda: model.generate_content(prompt))
            return response.text.strip()
        except Exception as e:
            logger.warning(f"Gemini generation failed: {e}. Falling back to Groq...")
    
    if groq_client:
        try:
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None,
                lambda: groq_client.chat.completions.create(
                    messages=[{"role": "user", "content": prompt}],
                    model="llama3-70b-8192",
                    temperature=0.2
                )
            )
            return response.choices[0].message.content.strip()
        except Exception as e:
            logger.error(f"Groq generation failed: {e}")
            
    return "AI generation unavailable. Please check API keys configuration."

# --- DOCUMENT PROCESSING PIPELINE ---

@app.post("/process-document")
async def process_document(request: DocumentProcessRequest, background_tasks: BackgroundTasks):
    """Start document processing asynchronously"""
    background_tasks.add_task(run_ingestion_pipeline, request.document_id, request.file_path, request.file_type, request.metadata)
    return {"message": "Document processing started", "document_id": request.document_id}

async def run_ingestion_pipeline(document_id: str, file_path: str, file_type: str, metadata: Dict[str, Any]):
    try:
        logger.info(f"Starting ingestion pipeline for document {document_id}")
        await update_status(document_id, "EXTRACTING_TEXT", 0.1)

        # 1. Fetch document version from database
        async with db_pool.acquire() as conn:
            version_row = await conn.fetchrow("""
                SELECT id FROM document.document_versions 
                WHERE document_id = $1 ORDER BY created_at DESC LIMIT 1
            """, uuid.UUID(document_id))
            if not version_row:
                raise Exception(f"No document version found for document {document_id}")
            document_version_id = version_row['id']

        # 2. Extract Text & OCR Fallback
        text_content = ""
        pages = []  # List of tuples: (page_no, text)
        
        if file_type.lower() == "pdf":
            pages = await extract_pdf_pages(file_path)
            text_content = "\n".join([p[1] for p in pages])
            
            # OCR Fallback
            if len(text_content.strip()) < 50:
                logger.info(f"Insufficient text extracted ({len(text_content)} chars). Running OCR fallback...")
                await update_status(document_id, "OCR_RUNNING", 0.3)
                pages = await ocr_pdf_pages(file_path)
                text_content = "\n".join([p[1] for p in pages])
        else:
            single_text = await extract_generic_text(file_path, file_type)
            pages = [(1, single_text)]
            text_content = single_text

        # 3. Convert to Markdown
        await update_status(document_id, "CONVERTING_TO_MARKDOWN", 0.5)
        markdown_content = convert_to_markdown(text_content, file_type)

        # 4. Extract Tables
        await update_status(document_id, "DETECTING_TABLES", 0.6)
        tables = await extract_tables(file_path, file_type)

        # 5. Token-Aware Chunking (500-800 tokens, 100 overlap)
        await update_status(document_id, "CHUNKING", 0.7)
        chunks = chunk_document_pages(pages)

        # 6. Extract Entities
        await update_status(document_id, "EXTRACTING_ENTITIES", 0.8)
        entities = extract_entities(chunks)

        # 7. Generate 1536-dimensional Gemini Embeddings
        await update_status(document_id, "GENERATING_EMBEDDINGS", 0.9)
        embeddings = []
        for chunk in chunks:
            emb = await get_gemini_embedding_1536(chunk["text"])
            embeddings.append(emb)

        # 8. Store Results in Database
        await update_status(document_id, "STORING_RESULTS", 0.95)
        await save_ingestion_results(
            document_id, document_version_id, markdown_content, pages, chunks, entities, embeddings, tables
        )

        # 9. Mark Completed
        await update_status(document_id, "COMPLETED", 1.0)
        logger.info(f"Document {document_id} processed successfully!")

    except Exception as e:
        logger.error(f"Failed to process document {document_id}: {e}", exc_info=True)
        await update_status(document_id, "FAILED", 0.0, str(e))

async def update_status(document_id: str, status: str, progress: float, error_message: str = None):
    """Updates document.documents and ingestion.processing_jobs"""
    if not db_pool:
        return
    try:
        doc_uuid = uuid.UUID(document_id)
        async with db_pool.acquire() as conn:
            await conn.execute("""
                UPDATE document.documents 
                SET status = $1, updated_at = NOW() 
                WHERE id = $2
            """, status, doc_uuid)
            
            await conn.execute("""
                INSERT INTO ingestion.processing_jobs (document_id, document_version_id, status, error_message, attempts, started_at)
                SELECT $1, id, $2, $3, 1, NOW()
                FROM document.document_versions
                WHERE document_id = $1
                ORDER BY created_at DESC LIMIT 1
                ON CONFLICT DO NOTHING
            """, doc_uuid, status, error_message)
    except Exception as e:
        logger.error(f"Error updating processing status: {e}")

# --- TEXT EXTRACTION UTILITIES ---

async def extract_pdf_pages(file_path: str) -> List[tuple]:
    pages = []
    try:
        with pdfplumber.open(file_path) as pdf:
            for i, page in enumerate(pdf.pages):
                text = page.extract_text() or ""
                pages.append((i + 1, text))
    except Exception as e:
        logger.warning(f"pdfplumber extraction failed: {e}. Trying PyPDF2...")
        try:
            with open(file_path, "rb") as f:
                reader = PyPDF2.PdfReader(f)
                for i, page in enumerate(reader.pages):
                    text = page.extract_text() or ""
                    pages.append((i + 1, text))
        except Exception as e2:
            logger.error(f"PyPDF2 also failed: {e2}")
    return pages

async def ocr_pdf_pages(file_path: str) -> List[tuple]:
    pages = []
    try:
        loop = asyncio.get_event_loop()
        # Convert pages to images in a background thread to prevent blocking
        images = await loop.run_in_executor(None, lambda: pdf2image.convert_from_path(file_path))
        for i, img in enumerate(images):
            text = await loop.run_in_executor(None, lambda image=img: pytesseract.image_to_string(image))
            pages.append((i + 1, text or ""))
    except Exception as e:
        logger.error(f"OCR pdf pages failed: {e}")
    return pages

async def extract_generic_text(file_path: str, file_type: str) -> str:
    try:
        if file_type.lower() in ["docx", "doc"]:
            doc = DocxDocument(file_path)
            return "\n".join([p.text for p in doc.paragraphs])
        elif file_type.lower() in ["xlsx", "xls"]:
            xl = pd.ExcelFile(file_path)
            sheets = []
            for name in xl.sheet_names:
                df = pd.read_excel(file_path, sheet_name=name)
                sheets.append(f"Sheet: {name}\n" + df.to_string(index=False))
            return "\n\n".join(sheets)
        elif file_type.lower() == "csv":
            df = pd.read_csv(file_path)
            return df.to_string(index=False)
        elif file_type.lower() in ["png", "jpg", "jpeg", "tiff", "bmp"]:
            loop = asyncio.get_event_loop()
            img = Image.open(file_path)
            text = await loop.run_in_executor(None, lambda: pytesseract.image_to_string(img))
            return text or ""
        else:
            # TXT / MD or other fallback
            with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
                return f.read()
    except Exception as e:
        logger.error(f"Generic text extraction failed for {file_path}: {e}")
        return ""

def convert_to_markdown(text: str, file_type: str) -> str:
    # Preserves basic structure and formats headers
    lines = text.split("\n")
    md_lines = []
    for line in lines:
        stripped = line.strip()
        if not stripped:
            md_lines.append("")
            continue
        if len(stripped) < 80 and stripped.isupper() and len(stripped.split()) <= 8:
            md_lines.append(f"## {stripped}")
        elif stripped.endswith(":") and len(stripped.split()) <= 4:
            md_lines.append(f"### {stripped}")
        else:
            md_lines.append(stripped)
    return "\n".join(md_lines)

async def extract_tables(file_path: str, file_type: str) -> List[Dict]:
    tables = []
    try:
        if file_type.lower() == "pdf":
            loop = asyncio.get_event_loop()
            dfs = await loop.run_in_executor(None, lambda: tabula.read_pdf(file_path, pages="all", multiple_tables=True))
            for i, df in enumerate(dfs):
                if not df.empty:
                    tables.append({
                        "table_id": f"table_{i}",
                        "data": df.to_dict("records"),
                        "columns": df.columns.tolist()
                    })
        elif file_type.lower() in ["xlsx", "xls"]:
            xl = pd.ExcelFile(file_path)
            for name in xl.sheet_names:
                df = pd.read_excel(file_path, sheet_name=name)
                if not df.empty:
                    tables.append({
                        "table_id": f"sheet_{name}",
                        "data": df.to_dict("records"),
                        "columns": df.columns.tolist()
                    })
        elif file_type.lower() == "csv":
            df = pd.read_csv(file_path)
            if not df.empty:
                tables.append({
                    "table_id": "csv_data",
                    "data": df.to_dict("records"),
                    "columns": df.columns.tolist()
                })
    except Exception as e:
        logger.warning(f"Table extraction failed for {file_path}: {e}")
    return tables

# --- CHUNKING & ENTITY EXTRACTION ---

def chunk_document_pages(pages: List[tuple]) -> List[Dict]:
    """
    Token-aware chunker.
    Groups lines/sentences to target chunks between 500 and 800 tokens, 
    with a 100 token overlap (approximated as 4 characters per token).
    """
    chunks = []
    chunk_size_chars = 600 * 4   # ~600 tokens
    overlap_chars = 100 * 4      # ~100 tokens
    
    for page_no, text in pages:
        text = text.strip()
        if not text:
            continue
            
        start = 0
        while start < len(text):
            end = start + chunk_size_chars
            if end < len(text):
                # Try to break at a sentence ending
                for i in range(end, max(start, end - 200), -1):
                    if text[i] in ".!?":
                        end = i + 1
                        break
            
            chunk_text = text[start:end].strip()
            if chunk_text:
                chunks.append({
                    "page_no": page_no,
                    "text": chunk_text,
                    "chunk_index": len(chunks)
                })
                
            start = end - overlap_chars
            if start >= len(text):
                break
                
    return chunks

def extract_entities(chunks: List[Dict]) -> List[Dict]:
    entities = []
    
    # Regexes for equipment tags, measurements, regulations, and dates
    tag_pattern = re.compile(r"\b[A-Z]+[-\s]*\d+[A-Z]*\b")
    date_pattern = re.compile(r"\b\d{1,2}[/\-\]\d{1,2}[/\-\]\d{2,4}\b|\b\d{1,2}\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov)[a-z]*\s+\d{2,4}\b", re.IGNORECASE)
    measurement_pattern = re.compile(r"\b\d+\.?\d*\s*(psi|bar|pa|kpa|mpa|°?[cfk]|rpm|hz|khz|mhz|mm|cm|m|km|in|ft)\b", re.IGNORECASE)
    regulation_pattern = re.compile(r"\b(Factory Act|OISD|PESO|ASME|OSHA|API\s*\d+)\b", re.IGNORECASE)

    for chunk in chunks:
        text = chunk["text"]
        
        # spaCy NER
        doc = nlp(text)
        for ent in doc.ents:
            entities.append({
                "chunk_index": chunk["chunk_index"],
                "entity_text": ent.text,
                "entity_type": ent.label_,
                "page_no": chunk["page_no"],
                "confidence": 0.8
            })

        # Custom Regex Matches
        for match in tag_pattern.finditer(text):
            entities.append({
                "chunk_index": chunk["chunk_index"],
                "entity_text": match.group(),
                "entity_type": "EQUIPMENT_TAG",
                "page_no": chunk["page_no"],
                "confidence": 0.95
            })
            
        for match in date_pattern.finditer(text):
            entities.append({
                "chunk_index": chunk["chunk_index"],
                "entity_text": match.group(),
                "entity_type": "DATE",
                "page_no": chunk["page_no"],
                "confidence": 0.85
            })

        for match in measurement_pattern.finditer(text):
            entities.append({
                "chunk_index": chunk["chunk_index"],
                "entity_text": match.group(),
                "entity_type": "MEASUREMENT",
                "page_no": chunk["page_no"],
                "confidence": 0.8
            })

        for match in regulation_pattern.finditer(text):
            entities.append({
                "chunk_index": chunk["chunk_index"],
                "entity_text": match.group(),
                "entity_type": "REGULATION",
                "page_no": chunk["page_no"],
                "confidence": 0.9
            })
            
    return entities

async def save_ingestion_results(document_id: str, document_version_id: uuid.UUID, markdown_content: str, 
                                 pages: List[tuple], chunks: List[Dict], entities: List[Dict], 
                                 embeddings: List[List[float]], tables: List[Dict]):
    """Saves output to PostgreSQL database schemas"""
    doc_uuid = uuid.UUID(document_id)
    
    async with db_pool.acquire() as conn:
        async with conn.transaction():
            # 1. Update document status & version markdown
            await conn.execute("""
                UPDATE document.document_versions 
                SET markdown_content = $1, ocr_confidence = 0.9, classification_confidence = 0.9, updated_at = NOW()
                WHERE id = $2
            """, markdown_content, document_version_id)
            
            await conn.execute("""
                UPDATE document.documents 
                SET status = 'COMPLETED', updated_at = NOW() 
                WHERE id = $1
            """, doc_uuid)
            
            # Fetch organization_id & plant_id
            doc_info = await conn.fetchrow("SELECT organization_id, plant_id FROM document.documents WHERE id = $1", doc_uuid)
            org_id = doc_info["organization_id"]
            plant_id = doc_info["plant_id"]

            # 2. Store Pages
            await conn.execute("DELETE FROM ingestion.document_pages WHERE document_id = $1", doc_uuid)
            for page_no, raw_text in pages:
                page_md = convert_to_markdown(raw_text, "pdf")
                await conn.execute("""
                    INSERT INTO ingestion.document_pages (document_id, document_version_id, page_no, raw_text, markdown_text, ocr_confidence)
                    VALUES ($1, $2, $3, $4, $5, 0.95)
                """, doc_uuid, document_version_id, page_no, raw_text, page_md)

            # 3. Store Chunks with 1536-dim embeddings
            await conn.execute("DELETE FROM ingestion.document_chunks WHERE document_id = $1", doc_uuid)
            chunk_uuids = []
            for i, chunk in enumerate(chunks):
                chunk_uuid = uuid.uuid4()
                chunk_uuids.append(chunk_uuid)
                await conn.execute("""
                    INSERT INTO ingestion.document_chunks (id, document_id, document_version_id, page_no, chunk_index, chunk_text, embedding, token_count)
                    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                """, chunk_uuid, doc_uuid, document_version_id, chunk["page_no"], chunk["chunk_index"], chunk["text"], embeddings[i], len(chunk["text"].split()))

            # 4. Store Entities (Mapping tag values as normalized profiles)
            await conn.execute("DELETE FROM graph.entities WHERE document_id = $1", doc_uuid)
            for ent in entities:
                c_idx = ent["chunk_index"]
                mapped_chunk_uuid = chunk_uuids[c_idx] if c_idx < len(chunk_uuids) else None
                
                await conn.execute("""
                    INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
                    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
                """, org_id, plant_id, doc_uuid, mapped_chunk_uuid, ent["entity_type"], ent["entity_text"], ent["entity_text"].upper().strip(), ent["confidence"], ent["page_no"])

                # If entity is an asset tag, automatically register/upsert in asset.assets
                if ent["entity_type"] == "EQUIPMENT_TAG":
                    asset_tag = ent["entity_text"].upper().strip()
                    await conn.execute("""
                        INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score)
                        VALUES ($1, $2, $3, $4, $5, 'Unit-1', 'MEDIUM', 50)
                        ON CONFLICT (plant_id, asset_tag) DO NOTHING
                    """, org_id, plant_id, asset_tag, f"Equipment {asset_tag}", "Asset")

            # 5. Store Tables as Entities
            for table in tables:
                table_str = json.dumps(table["data"])
                await conn.execute("""
                    INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
                    VALUES ($1, $2, $3, NULL, 'TABLE', $4, $5, 0.95, 1)
                """, org_id, plant_id, doc_uuid, f"Table {table['table_id']}", table_str)

# --- COGNITIVE SERVICES (RAG & COPILOT) ---

@app.post("/query")
async def rag_query(request: CopilotQueryRequest):
    """Executes hybrid pgvector search and synthesizes answers with citations"""
    if not db_pool:
        raise HTTPException(status_code=500, detail="Database connection unavailable")

    try:
        # Generate 1536-dimensional query embedding
        query_embedding = await get_gemini_embedding_1536(request.question)
        
        # Parse filters
        asset_filter = request.filters.get("assetTag") if request.filters else None
        
        # 1. Retrieve Candidate Chunks (pgvector Cosine Distance <->)
        async with db_pool.acquire() as conn:
            org_id = await conn.fetchval(
                "SELECT organization_id FROM identity.plants WHERE id = $1",
                uuid.UUID(request.plantId)
            )
            if not org_id:
                org_row = await conn.fetchrow(
                    "SELECT organization_id FROM identity.plants WHERE id = $1",
                    uuid.UUID(request.plantId)
                )
                if org_row:
                    org_id = org_row["organization_id"]
            if not org_id:
                raise HTTPException(status_code=404, detail="Plant not found")

            # We filter by plant_id of the document
            chunks_rows = await conn.fetch("""
                SELECT c.id, c.document_id, c.page_no, c.chunk_text, d.title,
                       (c.embedding <=> $1) as distance
                FROM ingestion.document_chunks c
                JOIN document.documents d ON c.document_id = d.id
                WHERE d.plant_id = $2
                ORDER BY distance ASC
                LIMIT 6
            """, query_embedding, uuid.UUID(request.plantId))
            
            # 2. Keyword/Exact Tag search fallback (if tag is queried or contained)
            keyword_chunks = []
            extracted_tags = re.findall(r"\b[A-Z]+[-\s]*\d+[A-Z]*\b", request.question.upper())
            if asset_filter:
                extracted_tags.append(asset_filter.upper())
                
            if extracted_tags:
                tag_queries = [f"%{tag}%" for tag in extracted_tags]
                for t_q in tag_queries:
                    k_rows = await conn.fetch("""
                        SELECT c.id, c.document_id, c.page_no, c.chunk_text, d.title, 0.0 as distance
                        FROM ingestion.document_chunks c
                        JOIN document.documents d ON c.document_id = d.id
                        WHERE d.plant_id = $1 AND (c.chunk_text ILIKE $2 OR d.title ILIKE $2)
                        LIMIT 3
                    """, uuid.UUID(request.plantId), t_q)
                    keyword_chunks.extend(k_rows)
            
            # Combine searches
            seen = set()
            combined_results = []
            for r in (keyword_chunks + list(chunks_rows)):
                if r["id"] not in seen:
                    seen.add(r["id"])
                    combined_results.append(r)
            
            # Sort combined results (exact keyword hits are prioritised)
            combined_results = combined_results[:6]
            
            if not combined_results:
                return {
                    "answer": "No relevant documentation or evidence was found for the query in this plant's database.",
                    "confidence": 0.0,
                    "citations": [],
                    "relatedAssets": [],
                    "missingInfo": []
                }

            # 3. Construct prompt
            context_blocks = []
            citations = []
            for idx, r in enumerate(combined_results):
                context_blocks.append(f"[{idx+1}] Doc: {r['title']} (Page {r['page_no']})\n{r['chunk_text']}")
                citations.append({
                    "documentId": str(r["document_id"]),
                    "documentTitle": r["title"],
                    "page": r["page_no"],
                    "snippet": r["chunk_text"][:200] + "..."
                })
                
            context_str = "\n\n".join(context_blocks)
            prompt = f"""
You are an expert industrial engineering AI. Answer the following technical question based ONLY on the provided document excerpts.
Every fact in your answer must cite the source excerpt index like [1] or [2] matching the provided texts.
If the information is not present, clearly state what information is missing.

Question: {request.question}

Excerpts:
{context_str}

Format your output exactly as:
ANSWER: <detailed answer citing [1], [2] etc>
CONFIDENCE: <estimated floating point score between 0.0 and 1.0>
MISSING_INFO: <comma separated details of any missing info or data gaps, or None>
"""
            llm_output = await generate_text_llm(prompt)
            
            # Parse output
            answer = "Unable to process query."
            confidence = 0.5
            missing_info = []
            
            try:
                ans_match = re.search(r"ANSWER:\s*(.*?)(?=CONFIDENCE:|$)", llm_output, re.DOTALL)
                conf_match = re.search(r"CONFIDENCE:\s*([\d\.]+)", llm_output)
                miss_match = re.search(r"MISSING_INFO:\s*(.*)", llm_output)
                
                if ans_match:
                    answer = ans_match.group(1).strip()
                if conf_match:
                    confidence = float(conf_match.group(1).strip())
                if miss_match:
                    info_text = miss_match.group(1).strip()
                    if info_text.lower() != "none" and info_text:
                        missing_info = [x.strip() for x in info_text.split(",") if x.strip()]
            except Exception as parse_e:
                logger.error(f"Error parsing LLM output: {parse_e}. Raw output: {llm_output}")
                answer = llm_output
            
            # Save query log to DB
            query_uuid = uuid.uuid4()
            await conn.execute("""
                INSERT INTO rag.queries (id, organization_id, plant_id, query_text, answer_text, confidence)
                VALUES ($1, $2, $3, $4, $5, $6)
            """, query_uuid, org_id, uuid.UUID(request.plantId), request.question, answer, confidence)

            for citation in citations:
                await conn.execute("""
                    INSERT INTO rag.citations (query_id, document_id, page_no, quoted_text)
                    VALUES ($1, $2, $3, $4)
                """, query_uuid, uuid.UUID(citation["documentId"]), citation["page"], citation["snippet"])

            return {
                "answer": answer,
                "confidence": confidence,
                "citations": citations,
                "relatedAssets": extracted_tags,
                "missingInfo": missing_info
            }

    except Exception as e:
        logger.error(f"RAG query execution failed: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))

# --- ROOT CAUSE ANALYSIS (RCA) SERVICE ---

@app.post("/rca")
async def generate_rca(request: RCAGenerateRequest):
    """Retrieves maintenance logs and work orders for an asset tag and drafts an LLM-powered RCA report"""
    if not db_pool:
        raise HTTPException(status_code=500, detail="Database connection unavailable")
        
    asset_tag = request.assetTag.upper().strip()
    try:
        async with db_pool.acquire() as conn:
            # Query all entities matching this asset tag to pull corresponding document contents
            rows = await conn.fetch("""
                SELECT DISTINCT c.chunk_text, d.title, d.created_at
                FROM graph.entities e
                JOIN ingestion.document_chunks c ON e.chunk_id = c.id
                JOIN document.documents d ON c.document_id = d.id
                WHERE e.normalized_value = $1
                ORDER BY d.created_at DESC
                LIMIT 10
            """, asset_tag)
            
            if not rows:
                return {
                    "summary": f"No historical maintenance records or manuals found for asset tag {asset_tag}.",
                    "probableCauses": ["Data deficiency"],
                    "recommendations": ["Upload asset manuals and past work orders to initiate RCA"],
                    "confidence": 0.0,
                    "citations": []
                }
                
            history_blocks = []
            for r in rows:
                history_blocks.append(f"Date: {r['created_at'].strftime('%Y-%m-%d')} | Doc: {r['title']}\n{r['chunk_text']}")
            
            history_str = "\n\n".join(history_blocks)
            prompt = f"""
You are an industrial reliability expert. Construct a Root Cause Analysis (RCA) report for the asset {asset_tag}.
Symptoms of failure: {request.failureDescription}

Here is the historical context of logs, inspection records, and manual entries for {asset_tag}:
{history_str}

Please generate an RCA with:
1. PROBLEM SUMMARY: A clear problem statement based on the logs.
2. TIMELINE: Trace the chronology of failure events.
3. PROBABLE CAUSES: Deduce likely causes using 5-Whys methodology.
4. RECOMMENDATIONS: Bullet points of actionable corrective actions.
5. CONFIDENCE SCORE: A decimal between 0.0 and 1.0.

Format your output exactly as:
SUMMARY: <detailed text>
PROBABLE_CAUSES: <comma separated causes>
RECOMMENDATIONS: <comma separated actions>
CONFIDENCE: <decimal value>
"""
            llm_output = await generate_text_llm(prompt)
            
            # Simple parses
            summary = "Failed to draft RCA summary."
            probable_causes = []
            recommendations = []
            confidence = 0.5
            
            try:
                sum_match = re.search(r"SUMMARY:\s*(.*?)(?=PROBABLE_CAUSES:|$)", llm_output, re.DOTALL)
                causes_match = re.search(r"PROBABLE_CAUSES:\s*(.*?)(?=RECOMMENDATIONS:|$)", llm_output, re.DOTALL)
                recs_match = re.search(r"RECOMMENDATIONS:\s*(.*?)(?=CONFIDENCE:|$)", llm_output, re.DOTALL)
                conf_match = re.search(r"CONFIDENCE:\s*([\d\.]+)", llm_output)
                
                if sum_match:
                    summary = sum_match.group(1).strip()
                if causes_match:
                    probable_causes = [x.strip() for x in causes_match.group(1).split(",") if x.strip()]
                if recs_match:
                    recommendations = [x.strip() for x in recs_match.group(1).split(",") if x.strip()]
                if conf_match:
                    confidence = float(conf_match.group(1).strip())
            except Exception as e:
                logger.error(f"Error parsing RCA output: {e}. Raw text: {llm_output}")
                summary = llm_output
                
            # Log to rca.reports table
            asset_row = await conn.fetchrow("SELECT id, organization_id FROM asset.assets WHERE asset_tag = $1 LIMIT 1", asset_tag)
            if asset_row:
                await conn.execute("""
                    INSERT INTO rca.reports (organization_id, asset_id, failure_summary, probable_causes, recommendations, confidence)
                    VALUES ($1, $2, $3, $4, $5, $6)
                """, asset_row["organization_id"], asset_row["id"], summary, json.dumps(probable_causes), json.dumps(recommendations), confidence)
                
            return {
                "summary": summary,
                "probableCauses": probable_causes,
                "recommendations": recommendations,
                "confidence": confidence,
                "citations": [{"documentTitle": r["title"]} for r in rows]
            }
            
    except Exception as e:
        logger.error(f"RCA generation failed: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))

# --- COMPLIANCE gap CHECKER ---

@app.get("/compliance")
async def audit_compliance(plantId: str):
    """Scans all assets for this plant and flags compliance gaps (missing checklists, overdue inspections)"""
    if not db_pool:
        raise HTTPException(status_code=500, detail="Database connection unavailable")
        
    try:
        plant_uuid = uuid.UUID(plantId)
        async with db_pool.acquire() as conn:
            # 1. Fetch all assets for this plant
            assets = await conn.fetch("SELECT id, asset_tag, asset_name FROM asset.assets WHERE plant_id = $1", plant_uuid)
            
            # Fetch all requirements
            requirements = await conn.fetch("SELECT id, title, requirement_type, frequency FROM compliance.requirements")
            
            if not requirements:
                # Insert default requirements if none exist
                default_reqs = [
                    ("Annual Pressure Vessel Test", "SAFETY", "yearly"),
                    ("Quarterly Fire Extinguisher Check", "SAFETY", "quarterly"),
                    ("Monthly Pump Mechanical Seal Leak Inspection", "MAINTENANCE", "monthly")
                ]
                # Fetch org ID
                org_row = await conn.fetchrow("SELECT organization_id FROM identity.plants WHERE id = $1", plant_uuid)
                org_id = org_row["organization_id"] if org_row else uuid.uuid4()
                
                for title, r_type, freq in default_reqs:
                    await conn.execute("""
                        INSERT INTO compliance.requirements (organization_id, plant_id, title, requirement_type, frequency)
                        VALUES ($1, $2, $3, $4, $5)
                    """, org_id, plant_uuid, title, r_type, freq)
                requirements = await conn.fetch("SELECT id, title, requirement_type, frequency FROM compliance.requirements WHERE plant_id = $1", plant_uuid)

            # 2. Check each asset against the requirements
            # We look in the graph.entities or document.documents for inspection reports
            gaps = []
            for asset in assets:
                asset_id = asset["id"]
                tag = asset["asset_tag"]
                
                # Check for "Inspection" or "Report" or "Certificate" documents mentioning this asset tag
                has_inspection = await conn.fetchval("""
                    SELECT COUNT(*) FROM graph.entities e
                    JOIN document.documents d ON e.document_id = d.id
                    WHERE e.normalized_value = $1 AND (d.document_type ILIKE '%inspection%' OR d.title ILIKE '%inspection%' OR d.title ILIKE '%report%')
                """, tag)
                
                if has_inspection == 0:
                    # Missing all inspections
                    for req in requirements:
                        # Log gap
                        description = f"No inspection evidence found for asset {tag} satisfying standard requirement '{req['title']}'."
                        await conn.execute("""
                            INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, requirement_id, gap_type, description, severity, status)
                            VALUES ((SELECT organization_id FROM asset.assets WHERE id = $1), $2, $1, $3, 'MISSING_EVIDENCE', $4, 'HIGH', 'OPEN')
                            ON CONFLICT DO NOTHING
                        """, asset_id, plant_uuid, req["id"], description)
                        
                        gaps.append({
                            "assetTag": tag,
                            "gapType": "MISSING_EVIDENCE",
                            "description": description,
                            "severity": "HIGH",
                            "status": "OPEN",
                            "standard": req["title"]
                        })
                else:
                    # Check for expiration of certificate if any document has an expiry tag
                    expired_docs = await conn.fetch("""
                        SELECT d.id, d.title FROM graph.entities e
                        JOIN document.documents d ON e.document_id = d.id
                        WHERE e.normalized_value = $1 AND d.title ILIKE '%expired%'
                    """, tag)
                    for ed in expired_docs:
                        description = f"Overdue certificate/expired document: '{ed['title']}' found associated with {tag}."
                        await conn.execute("""
                            INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, gap_type, description, severity, status, evidence_document_id)
                            VALUES ((SELECT organization_id FROM asset.assets WHERE id = $1), $2, $1, 'EXPIRED_CERTIFICATE', $3, 'CRITICAL', 'OPEN', $4)
                            ON CONFLICT DO NOTHING
                        """, asset_id, plant_uuid, description, ed["id"])
                        gaps.append({
                            "assetTag": tag,
                            "gapType": "EXPIRED_CERTIFICATE",
                            "description": description,
                            "severity": "CRITICAL",
                            "status": "OPEN",
                            "standard": "Regulatory Certificate"
                        })
            
            return gaps
            
    except Exception as e:
        logger.error(f"Compliance audit failed: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
