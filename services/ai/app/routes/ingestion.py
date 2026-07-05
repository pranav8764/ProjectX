import asyncio
import logging
import uuid
import json
import re
from datetime import datetime
from typing import Dict, Any, Optional, List

import asyncpg
import numpy as np
from fastapi import APIRouter, BackgroundTasks, HTTPException
from pydantic import BaseModel

import PyPDF2
import pdfplumber
from docx import Document as DocxDocument
import pandas as pd
from PIL import Image
import pytesseract
import pdf2image
import tabula

from app.config import logger, nlp
from app.schemas import DocumentProcessRequest
import app.database as database
from app.utils.ai_clients import get_gemini_embedding_1536
from app.utils.helpers import optional_uuid

router = APIRouter()

@router.post("/process-document")
async def process_document(request: DocumentProcessRequest, background_tasks: BackgroundTasks):
    """Start document processing asynchronously"""
    if not database.db_pool:
        raise HTTPException(status_code=500, detail="Database connection pool is not initialized")
    background_tasks.add_task(run_ingestion_pipeline, request.document_id, request.file_path, request.file_type, request.metadata)
    return {"message": "Document processing started", "document_id": request.document_id}

async def run_ingestion_pipeline(document_id: str, file_path: str, file_type: str, metadata: Dict[str, Any]):
    if not database.db_pool:
        logger.error("Cannot process document %s because database pool is unavailable", document_id)
        return

    document_version_id = None
    try:
        logger.info(f"Starting ingestion pipeline for document {document_id}")

        # 1. Fetch document version from database
        async with database.db_pool.acquire() as conn:
            version_row = await conn.fetchrow("""
                SELECT COALESCE(d.current_version_id, v.id) AS id
                FROM document.documents d
                LEFT JOIN LATERAL (
                    SELECT id
                    FROM document.document_versions
                    WHERE document_id = d.id
                    ORDER BY created_at DESC
                    LIMIT 1
                ) v ON true
                WHERE d.id = $1
            """, uuid.UUID(document_id))
            if not version_row or not version_row["id"]:
                raise Exception(f"No document version found for document {document_id}")
            document_version_id = version_row['id']

        await update_status(document_id, "EXTRACTING_TEXT", 0.1, document_version_id=document_version_id)

        # 2. Extract Text & OCR Fallback
        text_content = ""
        pages = []  # List of tuples: (page_no, text)

        if file_type.lower() == "pdf":
            pages = await extract_pdf_pages(file_path)

            # Mixed content & Garbled text
            final_pages = []
            ocr_needed_pages = []
            for p_idx, text in pages:
                text_strip = text.strip()
                alnum_count = sum(c.isalnum() for c in text_strip)
                alnum_ratio = alnum_count / len(text_strip) if len(text_strip) > 0 else 1.0

                if len(text_strip) < 50 or (len(text_strip) > 0 and alnum_ratio < 0.5):
                    ocr_needed_pages.append(p_idx)
                else:
                    final_pages.append((p_idx, text))

            if ocr_needed_pages:
                logger.info(f"Running OCR fallback for {len(ocr_needed_pages)} pages...")
                await update_status(document_id, "OCR_RUNNING", 0.3, document_version_id=document_version_id)
                ocr_results = await ocr_pdf_pages(file_path, pages_to_ocr=ocr_needed_pages)
                for p in ocr_results:
                    final_pages.append(p)

            final_pages.sort(key=lambda x: x[0])
            pages = final_pages
            text_content = "\n".join([p[1] for p in pages])
        else:
            single_text = await extract_generic_text(file_path, file_type)
            pages = [(1, single_text)]
            text_content = single_text

        # 3. Convert to Markdown
        await update_status(document_id, "CONVERTING_TO_MARKDOWN", 0.5, document_version_id=document_version_id)
        markdown_content = convert_to_markdown(text_content, file_type)

        await update_status(document_id, "CLASSIFYING", 0.55, document_version_id=document_version_id)

        # 4. Extract Tables
        await update_status(document_id, "DETECTING_TABLES", 0.6, document_version_id=document_version_id)
        tables = await extract_tables(file_path, file_type)

        # 5. Token-Aware Chunking (500-800 tokens, 100 overlap)
        await update_status(document_id, "CHUNKING", 0.7, document_version_id=document_version_id)
        chunks = chunk_document_pages(pages)
        partial_reasons = []
        if not text_content.strip():
            partial_reasons.append("No extractable text was found after native extraction and OCR fallback.")
        if text_content.strip() and not chunks:
            partial_reasons.append("Extracted text could not be converted into searchable chunks.")
        if not tables and file_type.lower() in {"xlsx", "xls", "csv"}:
            partial_reasons.append("No structured table data was detected in the spreadsheet or CSV file.")

        # 6. Extract Entities
        await update_status(document_id, "EXTRACTING_ENTITIES", 0.8, document_version_id=document_version_id)
        entities = extract_entities(chunks)

        await update_status(document_id, "GENERATING_EMBEDDINGS", 0.9, document_version_id=document_version_id)
        from app.utils.ai_clients import get_gemini_embeddings_1536_batch
        chunk_texts = [chunk["text"] for chunk in chunks]
        org_id = metadata.get("organization_id") or metadata.get("organizationId") if isinstance(metadata, dict) else None
        embeddings = await get_gemini_embeddings_1536_batch(chunk_texts, org_id=org_id)

        # 8. Store Results in Database
        await update_status(document_id, "STORING_RESULTS", 0.95, document_version_id=document_version_id)
        await save_ingestion_results(
            document_id, document_version_id, markdown_content, pages, chunks, entities, embeddings, tables
        )

        await update_status(document_id, "BUILDING_GRAPH", 0.98, document_version_id=document_version_id)

        # 9. Mark Completed
        if not text_content.strip() and not tables and not entities:
            error_msg = "Corrupt or empty document: no text, tables, or entities extracted."
            await update_status(document_id, "FAILED", 0.0, error_msg, document_version_id=document_version_id)
            logger.error(f"Document {document_id} failed processing: {error_msg}")
            return

        if partial_reasons:
            partial_reason = " ".join(partial_reasons)
            await update_status(document_id, "PARTIAL_SUCCESS", 1.0, partial_reason, document_version_id=document_version_id)
            logger.warning(f"Document {document_id} partially processed: {partial_reason}")
            return
        await update_status(document_id, "COMPLETED", 1.0, document_version_id=document_version_id)
        logger.info(f"Document {document_id} processed successfully!")

    except Exception as e:
        logger.error(f"Failed to process document {document_id}: {e}", exc_info=True)
        await update_status(document_id, "FAILED", 0.0, str(e), document_version_id=document_version_id)

async def update_status(
    document_id: str,
    status: str,
    progress: float,
    error_message: str = None,
    document_version_id: Optional[uuid.UUID] = None,
):
    """Updates document.documents and ingestion.processing_jobs"""
    if not database.db_pool:
        return
    try:
        doc_uuid = uuid.UUID(document_id)
        version_uuid = uuid.UUID(str(document_version_id)) if document_version_id else None
        terminal_status = status in {"COMPLETED", "FAILED", "PARTIAL_SUCCESS", "ARCHIVED"}
        increment_attempts = status == "EXTRACTING_TEXT"
        async with database.db_pool.acquire() as conn:
            if not version_uuid:
                version_row = await conn.fetchrow("""
                    SELECT COALESCE(d.current_version_id, v.id) AS id
                    FROM document.documents d
                    LEFT JOIN LATERAL (
                        SELECT id
                        FROM document.document_versions
                        WHERE document_id = d.id
                        ORDER BY created_at DESC
                        LIMIT 1
                    ) v ON true
                    WHERE d.id = $1
                """, doc_uuid)
                if not version_row or not version_row["id"]:
                    logger.warning("Cannot update processing status for document %s without a document version", document_id)
                    return
                version_uuid = version_row["id"]

            await conn.execute("""
                UPDATE document.documents
                SET status = $1, updated_at = NOW()
                WHERE id = $2
                  AND (current_version_id IS NULL OR current_version_id = $3)
            """, status, doc_uuid, version_uuid)

            await conn.execute("""
                INSERT INTO ingestion.processing_jobs (
                    document_id, document_version_id, status, error_message, attempts, progress,
                    last_attempted_at, next_retry_at, locked_at, locked_by, metadata_json,
                    started_at, completed_at, created_at, updated_at
                )
                VALUES (
                    $1, $2, $3, $4, CASE WHEN $6 THEN 1 ELSE 0 END, $5,
                    CASE WHEN $6 THEN NOW() ELSE NULL END,
                    CASE
                        WHEN $3 = 'QUEUED' THEN NOW()
                        WHEN $3 = 'FAILED' THEN NOW() + (INTERVAL '1 minute' * POWER(2, CASE WHEN $6 THEN 1 ELSE 0 END))
                        ELSE NULL
                    END,
                    NULL, NULL, jsonb_build_object('lastTransition', $3, 'source', 'ai_service'),
                    NOW(), CASE WHEN $7 THEN NOW() ELSE NULL END, NOW(), NOW()
                )
                ON CONFLICT (document_id, document_version_id)
                DO UPDATE SET
                    status = EXCLUDED.status,
                    error_message = EXCLUDED.error_message,
                    progress = EXCLUDED.progress,
                    attempts = ingestion.processing_jobs.attempts + CASE WHEN $6 THEN 1 ELSE 0 END,
                    last_attempted_at = CASE WHEN $6 THEN NOW() ELSE ingestion.processing_jobs.last_attempted_at END,
                    next_retry_at = CASE
                        WHEN EXCLUDED.status = 'QUEUED' THEN NOW()
                        WHEN EXCLUDED.status = 'FAILED' THEN NOW() + (INTERVAL '1 minute' * POWER(2, ingestion.processing_jobs.attempts + CASE WHEN $6 THEN 1 ELSE 0 END))
                        ELSE NULL
                    END,
                    locked_at = NULL,
                    locked_by = NULL,
                    metadata_json = ingestion.processing_jobs.metadata_json || EXCLUDED.metadata_json,
                    started_at = CASE WHEN $6 THEN NOW() ELSE COALESCE(ingestion.processing_jobs.started_at, NOW()) END,
                    completed_at = CASE WHEN $7 THEN NOW() ELSE NULL END,
                    updated_at = NOW()
            """, doc_uuid, version_uuid, status, error_message, progress, increment_attempts, terminal_status)
    except Exception as e:
        logger.error(f"Error updating processing status: {e}")

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

async def ocr_pdf_pages(file_path: str, pages_to_ocr: Optional[List[int]] = None) -> List[tuple]:
    pages = []
    try:
        loop = asyncio.get_event_loop()
        info = await loop.run_in_executor(None, lambda: pdf2image.pdfinfo_from_path(file_path))
        num_pages = int(info["Pages"])

        for i in range(1, num_pages + 1):
            if pages_to_ocr and i not in pages_to_ocr:
                continue
            images = await loop.run_in_executor(
                None,
                lambda page=i: pdf2image.convert_from_path(file_path, first_page=page, last_page=page)
            )
            if images:
                img = images[0]
                text = await loop.run_in_executor(None, lambda image=img: pytesseract.image_to_string(image))
                pages.append((i, text or ""))
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
            with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
                return f.read()
    except Exception as e:
        logger.error(f"Generic text extraction failed for {file_path}: {e}")
        return ""

def convert_to_markdown(text: str, file_type: str) -> str:
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

def chunk_document_pages(pages: List[tuple]) -> List[Dict]:
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

    tag_pattern = re.compile(r"\b(?!(?:VERSION|PAGE|REV|TABLE|FIG|FIGURE)\b)[A-Z]+[-\s]*\d+[A-Z]*\b")
    date_pattern = re.compile(r"\b\d{1,2}[/\-\]\d{1,2}[/\-\]\d{2,4}\b|\b\d{1,2}\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov)[a-z]*\s+\d{2,4}\b", re.IGNORECASE)
    measurement_pattern = re.compile(r"\b\d+\.?\d*\s*(psi|bar|pa|kpa|mpa|°?[cfk]|rpm|hz|khz|mhz|mm|cm|m|km|in|ft)\b", re.IGNORECASE)
    regulation_pattern = re.compile(r"\b(Factory Act|OISD|PESO|ASME|OSHA|API\s*\d+)\b", re.IGNORECASE)
    failure_pattern = re.compile(r"\b(leak(?:age)?|overheat(?:ing)?|vibration|bearing failure|seal failure|cavitation|corrosion|crack|trip|shutdown|fault)\b", re.IGNORECASE)
    action_pattern = re.compile(r"\b(replace(?:d|ment)?|inspect(?:ed|ion)?|repair(?:ed)?|clean(?:ed|ing)?|calibrat(?:ed|ion)|lubricat(?:ed|ion)|tighten(?:ed)?|align(?:ed|ment)?)\b", re.IGNORECASE)
    severity_pattern = re.compile(r"\b(low|medium|high|critical)\b", re.IGNORECASE)
    location_pattern = re.compile(r"\b(?:Unit|Area|Boiler Area|Pump House|Compressor Bay|Plant|Line)[-\s]?\d*[A-Z]*\b", re.IGNORECASE)

    for chunk in chunks:
        text = chunk["text"]
        doc = nlp(text)
        for ent in doc.ents:
            entities.append({
                "chunk_index": chunk["chunk_index"],
                "entity_text": ent.text,
                "entity_type": ent.label_,
                "page_no": chunk["page_no"],
                "confidence": 0.8
            })

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

        for pattern, entity_type, confidence in [
            (failure_pattern, "FAILURE_TYPE", 0.82),
            (action_pattern, "MAINTENANCE_ACTION", 0.78),
            (severity_pattern, "SEVERITY", 0.78),
            (location_pattern, "LOCATION", 0.76),
        ]:
            for match in pattern.finditer(text):
                entities.append({
                    "chunk_index": chunk["chunk_index"],
                    "entity_text": match.group(),
                    "entity_type": entity_type,
                    "page_no": chunk["page_no"],
                    "confidence": confidence
                })

    return entities

def relationship_for_document_type(document_type: str) -> str:
    normalized = (document_type or "").upper()
    if "MANUAL" in normalized:
        return "HAS_MANUAL"
    if "SOP" in normalized or "SAFETY" in normalized:
        return "APPLIES_TO"
    if "INSPECTION" in normalized:
        return "HAS_INSPECTION"
    if "WORK" in normalized or "MAINTENANCE" in normalized:
        return "HAS_WORK_ORDER"
    if "COMPLIANCE" in normalized or "AUDIT" in normalized:
        return "HAS_EVIDENCE"
    return "MENTIONED_IN"

def relationship_for_entity_type(entity_type: str) -> Optional[str]:
    return {
        "FAILURE_TYPE": "HAS_FAILURE",
        "MAINTENANCE_ACTION": "HAS_RECOMMENDATION",
        "REGULATION": "GOVERNED_BY",
        "DATE": "INSPECTED_ON",
        "LOCATION": "LOCATED_IN",
        "SEVERITY": "HAS_STATUS",
        "MEASUREMENT": "HAS_STATUS",
    }.get(entity_type)

async def save_ingestion_results(document_id: str, document_version_id: uuid.UUID, markdown_content: str,
                                 pages: List[tuple], chunks: List[Dict], entities: List[Dict],
                                 embeddings: List[List[float]], tables: List[Dict]):
    """Saves output to PostgreSQL database schemas"""
    doc_uuid = uuid.UUID(document_id)

    async with database.db_pool.acquire() as conn:
        async with conn.transaction():
            # 1. Update version markdown. The final document status is owned by update_status.
            await conn.execute("""
                UPDATE document.document_versions
                SET markdown_content = $1, ocr_confidence = 0.9, classification_confidence = 0.9, updated_at = NOW()
                WHERE id = $2
            """, markdown_content, document_version_id)

            # Fetch organization_id & plant_id
            doc_info = await conn.fetchrow("SELECT organization_id, plant_id, title, document_type FROM document.documents WHERE id = $1", doc_uuid)
            org_id = doc_info["organization_id"]
            plant_id = doc_info["plant_id"]
            document_title = doc_info.get("title") or str(doc_uuid)
            document_type = (doc_info.get("document_type") or "document").upper()

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

            chunk_entities = {}
            document_entity_id = uuid.uuid4()
            await conn.execute("""
                INSERT INTO graph.entities (id, organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
                VALUES ($1, $2, $3, $4, NULL, 'DOCUMENT', $5, $6, 1.0, NULL)
            """, document_entity_id, org_id, plant_id, doc_uuid, document_title, document_type)

            for ent in entities:
                c_idx = ent["chunk_index"]
                mapped_chunk_uuid = chunk_uuids[c_idx] if c_idx < len(chunk_uuids) else None
                entity_id = uuid.uuid4()

                await conn.execute("""
                    INSERT INTO graph.entities (id, organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
                    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
                """, entity_id, org_id, plant_id, doc_uuid, mapped_chunk_uuid, ent["entity_type"], ent["entity_text"], ent["entity_text"].upper().strip(), ent["confidence"], ent["page_no"])

                if mapped_chunk_uuid:
                    if mapped_chunk_uuid not in chunk_entities:
                        chunk_entities[mapped_chunk_uuid] = []
                    chunk_entities[mapped_chunk_uuid].append({
                        "id": entity_id,
                        "type": ent["entity_type"],
                        "text": ent["entity_text"],
                    })

                # If entity is an asset tag, automatically register/upsert in asset.assets
                if ent["entity_type"] == "EQUIPMENT_TAG":
                    asset_tag = ent["entity_text"].upper().strip()
                    await conn.execute("""
                        INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type)
                        VALUES ($1, $2, $3, $4, $5)
                        ON CONFLICT (plant_id, asset_tag) DO NOTHING
                    """, org_id, plant_id, asset_tag, f"Equipment {asset_tag}", "Asset")

            # 4.1 Store semantic and co-occurrence relationships for entities in the same chunk
            await conn.execute("DELETE FROM graph.relationships WHERE evidence_document_id = $1", doc_uuid)
            for chunk_uuid, ents in chunk_entities.items():
                asset_entities = [ent for ent in ents if ent["type"] == "EQUIPMENT_TAG"]
                for asset_ent in asset_entities:
                    await conn.execute("""
                        INSERT INTO graph.relationships (organization_id, source_entity_id, target_entity_id, relationship_type, confidence, evidence_document_id, evidence_chunk_id)
                        VALUES ($1, $2, $3, $4, 0.9, $5, $6)
                    """, org_id, asset_ent["id"], document_entity_id, relationship_for_document_type(document_type), doc_uuid, chunk_uuid)

                    for target_ent in ents:
                        if target_ent["id"] == asset_ent["id"]:
                            continue
                        relationship_type = relationship_for_entity_type(target_ent["type"])
                        if relationship_type:
                            await conn.execute("""
                                INSERT INTO graph.relationships (organization_id, source_entity_id, target_entity_id, relationship_type, confidence, evidence_document_id, evidence_chunk_id)
                                VALUES ($1, $2, $3, $4, 0.82, $5, $6)
                            """, org_id, asset_ent["id"], target_ent["id"], relationship_type, doc_uuid, chunk_uuid)

                for i in range(len(ents)):
                    for j in range(i + 1, len(ents)):
                        await conn.execute("""
                            INSERT INTO graph.relationships (organization_id, source_entity_id, target_entity_id, relationship_type, confidence, evidence_document_id, evidence_chunk_id)
                            VALUES ($1, $2, $3, 'CO_OCCURS_WITH', 0.8, $4, $5)
                        """, org_id, ents[i]["id"], ents[j]["id"], doc_uuid, chunk_uuid)

            # 5. Store Tables as Entities
            for table in tables:
                table_str = json.dumps(table["data"])
                await conn.execute("""
                    INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
                    VALUES ($1, $2, $3, NULL, 'TABLE', $4, $5, 0.95, 1)
                """, org_id, plant_id, doc_uuid, f"Table {table['table_id']}", table_str)
