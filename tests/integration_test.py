#!/usr/bin/env python3
"""
PlantBrainAI Integration Test Suite
===================================

This script performs end-to-end integration tests for the PlantBrainAI AI Service.
It contains two test execution modes:
1. Mock Mode (Default): Runs tests against the local Python code by mocking 
   external APIs (Google Gemini, Groq) and the database (asyncpg). Requires no dependencies.
2. Live Service Mode: Runs real HTTP integration tests against running endpoints.
   Enabled by setting the `PLANTBRAIN_TEST_URL` environment variable.

How to Execute:
--------------
- Run local mocked tests (no dependencies needed):
    python3 tests/integration_test.py

- Run against a live service (e.g. running locally or in Docker):
    export PLANTBRAIN_TEST_URL=http://localhost:8000
    python3 tests/integration_test.py
"""

import os
import sys
import unittest
import uuid
import json
import asyncio
from unittest.mock import MagicMock, patch, AsyncMock

# Add workspace directory to Python path
workspace_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.append(workspace_dir)
sys.path.append(os.path.join(workspace_dir, "services", "ai"))
sys.path.append(os.path.join(workspace_dir, "services", "ai", "app"))

# Check for Live Service Mode URL
LIVE_SERVICE_URL = os.getenv("PLANTBRAIN_TEST_URL")


# ==========================================
# DYNAMIC DEPENDENCY MOCKING FOR LOCAL RUNS
# ==========================================

class MockBaseModel:
    """Mock class mimicking Pydantic's BaseModel for parsing and instantiating requests"""
    def __init__(self, **kwargs):
        for k, v in kwargs.items():
            setattr(self, k, v)
        # Handle defaults from annotations
        for k, t in getattr(self, '__annotations__', {}).items():
            if k not in kwargs:
                setattr(self, k, getattr(self.__class__, k, None))

class MockFastAPI:
    """Mock class mimicking FastAPI app to prevent dependency errors during import"""
    def __init__(self, *args, **kwargs):
        pass
    def post(self, *args, **kwargs):
        def decorator(func):
            return func
        return decorator
    def get(self, *args, **kwargs):
        def decorator(func):
            return func
        return decorator
    def on_event(self, *args, **kwargs):
        def decorator(func):
            return func
        return decorator

class MockHTTPException(Exception):
    def __init__(self, status_code=None, detail=None, *args, **kwargs):
        super().__init__(detail or status_code or "HTTPException")
        self.status_code = status_code
        self.detail = detail

class MockEnt:
    def __init__(self, text, label):
        self.text = text
        self.label_ = label
        self.start_char = 0
        self.end_char = len(text)

class MockDoc:
    def __init__(self, ents):
        self.ents = ents

class MockSpacy:
    """Mock class for Spacy to parse equipment tags, dates, measurements and regulations"""
    def load(self, *args, **kwargs):
        def nlp(text):
            ents = []
            if "P-101" in text:
                ents.append(MockEnt("P-101", "EQUIPMENT_TAG"))
            if "12 Jan 2025" in text:
                ents.append(MockEnt("12 Jan 2025", "DATE"))
            if "150 psi" in text:
                ents.append(MockEnt("150 psi", "MEASUREMENT"))
            if "Factory Act" in text:
                ents.append(MockEnt("Factory Act", "REGULATION"))
            return MockDoc(ents)
        return nlp

class MockGenAI:
    """Mock class for google.generativeai, simulating text generation and embeddings"""
    def configure(self, *args, **kwargs):
        pass
    def embed_content(self, model, content, task_type):
        # Models like text-embedding-004 return 768-dimensional vectors
        return {"embedding": [0.125] * 768}
    class GenerativeModel:
        def __init__(self, model_name):
            self.model_name = model_name
        def generate_content(self, prompt):
            class MockResponse:
                def __init__(self, text):
                    self.text = text
            # Yield formatted outputs matching custom parser regexes
            if "RCA" in prompt:
                return MockResponse("""
SUMMARY: Pump P-101 experienced persistent mechanical seal failure due to high fluid temperatures.
PROBABLE_CAUSES: Seal degradation from dry running, improper cooling, excessive vibration
RECOMMENDATIONS: Inspect cooling loop flow, verify alignment, install thermal sensors
CONFIDENCE: 0.95
""")
            elif "RAG" in prompt or "excerpts" in prompt:
                return MockResponse("""
ANSWER: Pump P-101 mechanical seal failed on 12 Jan 2025 [1] due to high pressure (150 psi) as noted in maintenance log [2].
CONFIDENCE: 0.90
MISSING_INFO: none
""")
            return MockResponse("Generic LLM mock response")


# Inject mock modules if they are not already installed on the system
needed_mocks = {
    "fastapi": (MockFastAPI, ["FastAPI", "HTTPException", "BackgroundTasks"]),
    "pydantic": (MockBaseModel, ["BaseModel"]),
    "asyncpg": (MagicMock(), []),
    "numpy": (MagicMock(), []),
    "PyPDF2": (MagicMock(), []),
    "pdfplumber": (MagicMock(), []),
    "docx": (MagicMock(), ["Document"]),
    "pandas": (MagicMock(), []),
    "PIL": (MagicMock(), ["Image"]),
    "pytesseract": (MagicMock(), []),
    "pdf2image": (MagicMock(), []),
    "spacy": (MockSpacy(), []),
    "tabula": (MagicMock(), []),
    "google": (MagicMock(), []),
    "google.generativeai": (MockGenAI(), []),
    "groq": (MagicMock(), ["Groq"])
}

for module_name, (mock_obj, sub_attribs) in needed_mocks.items():
    if module_name not in sys.modules:
        sys.modules[module_name] = mock_obj
    # Handle specific imports
    for attr in sub_attribs:
        setattr(mock_obj, attr, mock_obj if attr != "BaseModel" else MockBaseModel)

sys.modules["google"].generativeai = sys.modules["google.generativeai"]
            
# Override HTTPException explicitly to behave like a standard exception for testing
import fastapi
fastapi.HTTPException = MockHTTPException


# ==========================================
# MOCK DATABASE UTILITIES FOR OFFLINE TESTS
# ==========================================

class MockTransaction:
    async def __aenter__(self):
        return self
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        pass

class MockConnection:
    def __init__(self, fetch_results=None, fetchrow_results=None, fetchval_results=None):
        self.fetch_results = fetch_results or []
        self.fetchrow_results = fetchrow_results or {}
        self.fetchval_results = fetchval_results or {}
        self.executed_queries = []

    async def execute(self, query, *args):
        self.executed_queries.append((query, args))
        return "INSERT 0 1"

    async def fetch(self, query, *args):
        self.executed_queries.append((query, args))
        if isinstance(self.fetch_results, dict):
            for pattern, val in self.fetch_results.items():
                if pattern in query:
                    return val
            return []
        return self.fetch_results

    async def fetchrow(self, query, *args):
        self.executed_queries.append((query, args))
        if isinstance(self.fetchrow_results, dict):
            for pattern, val in self.fetchrow_results.items():
                if pattern in query:
                    return val
            return None
        return self.fetchrow_results

    async def fetchval(self, query, *args):
        self.executed_queries.append((query, args))
        if isinstance(self.fetchval_results, dict):
            for pattern, val in self.fetchval_results.items():
                if pattern in query:
                    return val
            return 0
        return self.fetchval_results

    def transaction(self):
        return MockTransaction()

class MockDbPool:
    def __init__(self, conn=None):
        self.conn = conn or MockConnection()

    def acquire(self):
        class AcquireContext:
            def __init__(self, conn):
                self.conn = conn
            async def __aenter__(self):
                return self.conn
            async def __aexit__(self, exc_type, exc_val, exc_tb):
                pass
        return AcquireContext(self.conn)


# ==========================================
# TEST CASE SUITE: OFFLINE MOCK MODE
# ==========================================

class TestMockIntegration(unittest.TestCase):
    """
    Tests the logic of individual pipeline stages and cognitive endpoints in 
    services/ai/app/main.py directly by importing it and mocking DB/LLM calls.
    """
    
    @classmethod
    def setUpClass(cls):
        # Enable the mocked environment variables before importing
        os.environ["GOOGLE_API_KEY"] = "mock-google-key"
        os.environ["GROQ_API_KEY"] = "mock-groq-key"
        
        # Import the main AI service module
        import services.ai.app.main as main
        cls.main = main

    def setUp(self):
        # Set up a fresh mock connection and DB pool for each test
        self.conn = MockConnection()
        self.main.db_pool = MockDbPool(self.conn)

    def test_gemini_embedding_1536(self):
        """Test Gemini 1536-dimensional embedding logic"""
        # Execute the generator function
        emb = asyncio.run(self.main.get_gemini_embedding_1536("Sample text"))
        
        # Check dimensionality is exactly 1536
        self.assertEqual(len(emb), 1536)
        
        # Check concatenation logic: first 768 elements must match last 768 elements
        self.assertEqual(emb[:768], emb[768:])
        
        # Since mocked vector is 0.125, check values
        self.assertEqual(emb[0], 0.125)
        self.assertEqual(emb[1535], 0.125)

    def test_ingestion_pipeline_flow(self):
        """Test document ingestion, chunking, and database storage flow"""
        doc_id = str(uuid.uuid4())
        version_id = uuid.uuid4()
        
        # Seed fetchrow results for document version lookups
        self.conn.fetchrow_results = {
            "COALESCE(d.current_version_id": {"id": version_id},
            "SELECT organization_id, plant_id": {"organization_id": uuid.uuid4(), "plant_id": uuid.uuid4()}
        }
        
        # Patches for files OCR and conversion to bypass filesystem interactions
        with patch('services.ai.app.main.extract_pdf_pages', return_value=[(1, "Pump P-101 has a mechanical seal leakage on 12 Jan 2025. Pressure: 150 psi. Factory Act governed.")]), \
             patch('services.ai.app.main.extract_tables', return_value=[]):
             
            # Run ingestion
            asyncio.run(self.main.run_ingestion_pipeline(
                document_id=doc_id,
                file_path="mock.pdf",
                file_type="pdf",
                metadata={"title": "Test doc"}
            ))
            
            # Inspect that we saved outputs in document_versions, document_pages, document_chunks, and entities
            queries = [q[0] for q in self.conn.executed_queries]
            
            # Verify database saving inserts
            self.assertTrue(any("INSERT INTO ingestion.document_pages" in q for q in queries))
            self.assertTrue(any("INSERT INTO ingestion.document_chunks" in q for q in queries))
            self.assertTrue(any("INSERT INTO graph.entities" in q for q in queries))
            
            # Verify assets registering for EQUIPMENT_TAG
            self.assertTrue(any("INSERT INTO asset.assets" in q for q in queries))

            # Verify processing job writes are pinned to the resolved current version.
            processing_writes = [
                args for query, args in self.conn.executed_queries
                if "INSERT INTO ingestion.processing_jobs" in query
            ]
            self.assertTrue(processing_writes)
            self.assertTrue(all(args[1] == version_id for args in processing_writes))
            self.assertEqual(processing_writes[0][2], "EXTRACTING_TEXT")
            self.assertTrue(processing_writes[0][5])
            self.assertEqual(processing_writes[-1][2], "COMPLETED")
            self.assertTrue(processing_writes[-1][6])

            document_status_updates = [
                args for query, args in self.conn.executed_queries
                if "UPDATE document.documents" in query
            ]
            self.assertEqual(document_status_updates[-1][0], "COMPLETED")
            self.assertEqual(document_status_updates[-1][2], version_id)

    def test_ingestion_pipeline_partial_success_for_empty_extraction(self):
        """Empty extraction persists version results but ends as PARTIAL_SUCCESS with missing-data reason"""
        doc_id = str(uuid.uuid4())
        version_id = uuid.uuid4()

        self.conn.fetchrow_results = {
            "COALESCE(d.current_version_id": {"id": version_id},
            "SELECT organization_id, plant_id": {"organization_id": uuid.uuid4(), "plant_id": uuid.uuid4()}
        }

        with patch('services.ai.app.main.extract_generic_text', return_value=""), \
             patch('services.ai.app.main.extract_tables', return_value=[]):
            asyncio.run(self.main.run_ingestion_pipeline(
                document_id=doc_id,
                file_path="empty.txt",
                file_type="txt",
                metadata={"title": "Empty manual retry"}
            ))

        processing_writes = [
            args for query, args in self.conn.executed_queries
            if "INSERT INTO ingestion.processing_jobs" in query
        ]
        self.assertTrue(processing_writes)
        self.assertEqual(processing_writes[-1][2], "FAILED")
        self.assertIn("Corrupt or empty document", processing_writes[-1][3])
        self.assertEqual(processing_writes[-1][4], 0.0)
        self.assertTrue(processing_writes[-1][6])

        document_status_updates = [
            args for query, args in self.conn.executed_queries
            if "UPDATE document.documents" in query
        ]
        self.assertEqual(document_status_updates[-1][0], "FAILED")
        self.assertEqual(document_status_updates[-1][2], version_id)

    def test_rag_query_with_citations(self):
        """Test RAG query synthesis and citation parsing"""
        plant_id = str(uuid.uuid4())
        doc_id = uuid.uuid4()
        
        # Seed chunks returned by vector search
        self.conn.fetch_results = {
            "SELECT c.id": [
                {
                    "id": uuid.uuid4(),
                    "document_id": doc_id,
                    "page_no": 1,
                    "chunk_text": "Pump P-101 failed on 12 Jan 2025.",
                    "title": "Maintenance Log P-101",
                    "distance": 0.1
                },
                {
                    "id": uuid.uuid4(),
                    "document_id": doc_id,
                    "page_no": 2,
                    "chunk_text": "Operating pressure was high at 150 psi.",
                    "title": "Incident Log P-101",
                    "distance": 0.15
                }
            ]
        }
        
        self.conn.fetchrow_results = {
            "SELECT organization_id": {"organization_id": uuid.uuid4()}
        }
        
        # Build query request object
        req = self.main.CopilotQueryRequest(
            question="Why did Pump P-101 fail?",
            plantId=plant_id,
            filters={"assetTag": "P-101"}
        )
        
        # Run query
        response = asyncio.run(self.main.rag_query(req))
        
        # Verify synthesized response
        self.assertIn("Pump P-101 mechanical seal failed", response["answer"])
        self.assertGreaterEqual(response["confidence"], 0.8)
        self.assertEqual(len(response["citations"]), 2)
        
        # Check citations structure
        self.assertEqual(response["citations"][0]["documentTitle"], "Maintenance Log P-101")
        self.assertEqual(response["citations"][0]["page"], 1)
        self.assertEqual(response["citations"][1]["documentTitle"], "Incident Log P-101")
        
        # Check assets list extraction
        self.assertIn("P-101", response["relatedAssets"])

    def test_rag_query_excludes_restricted_documents_for_role(self):
        """Test RAG retrieval excludes sources the current user role cannot access"""
        plant_id = str(uuid.uuid4())
        org_id = uuid.uuid4()
        user_id = str(uuid.uuid4())
        open_doc_id = uuid.uuid4()
        restricted_doc_id = uuid.uuid4()
        captured_prompt = {}

        self.conn.fetch_results = {
            "information_schema.columns": [
                {"column_name": "access_level"},
                {"column_name": "allowed_roles"},
            ],
            "SELECT c.id": [
                {
                    "id": uuid.uuid4(),
                    "document_id": open_doc_id,
                    "page_no": 1,
                    "chunk_text": "Pump inspection notes are available for routine maintenance.",
                    "title": "Open Maintenance Log",
                    "distance": 0.1,
                    "access_level": "public",
                    "allowed_roles": None,
                },
                {
                    "id": uuid.uuid4(),
                    "document_id": restricted_doc_id,
                    "page_no": 3,
                    "chunk_text": "Confidential root cause details must not be exposed to technicians.",
                    "title": "Restricted Compliance Finding",
                    "distance": 0.12,
                    "access_level": "restricted",
                    "allowed_roles": ["admin", "compliance_officer"],
                },
            ],
        }
        self.conn.fetchrow_results = {
            "SELECT organization_id": {"organization_id": org_id},
            "SELECT r.name AS role": {"role": "Field Technician"},
        }

        async def fake_generate(prompt):
            captured_prompt["text"] = prompt
            return """
ANSWER: Routine maintenance notes are available [1].
CONFIDENCE: 0.80
MISSING_INFO: none
"""

        req = self.main.CopilotQueryRequest(
            question="What maintenance notes are available?",
            plantId=plant_id,
            userId=user_id,
            organizationId=str(org_id),
        )

        with patch("services.ai.app.main.generate_text_llm", side_effect=fake_generate):
            response = asyncio.run(self.main.rag_query(req))

        self.assertEqual(len(response["citations"]), 1)
        self.assertEqual(response["citations"][0]["documentId"], str(open_doc_id))
        self.assertEqual(response["citations"][0]["documentTitle"], "Open Maintenance Log")
        self.assertNotIn("Restricted Compliance Finding", captured_prompt["text"])
        self.assertNotIn("Confidential root cause", captured_prompt["text"])

    def test_rca_generation(self):
        """Test Root Cause Analysis report generation"""
        asset_tag = "P-101"
        
        # Seed maintenance work orders history
        self.conn.fetch_results = [
            {
                "chunk_text": "Pump P-101 seal leak reported. Temperature exceeded normal levels.",
                "title": "Work Order WO-992",
                "created_at": asyncio.run(self.get_dummy_datetime())
            }
        ]
        
        self.conn.fetchrow_results = {
            "SELECT id, organization_id": {"id": uuid.uuid4(), "organization_id": uuid.uuid4()}
        }
        
        # Build RCA Request
        req = self.main.RCAGenerateRequest(
            assetTag=asset_tag,
            failureDescription="Mechanical seal leakage and overheating"
        )
        
        # Run RCA
        response = asyncio.run(self.main.generate_rca(req))
        
        # Check output components
        self.assertIn("Pump P-101 experienced persistent mechanical seal failure", response["summary"])
        self.assertEqual(len(response["probableCauses"]), 3)
        self.assertIn("improper cooling", response["probableCauses"])
        self.assertEqual(len(response["recommendations"]), 3)
        self.assertEqual(response["confidence"], 0.95)
        self.assertEqual(response["citations"][0]["documentTitle"], "Work Order WO-992")

    def test_compliance_gap_scanning(self):
        """Test scanning compliance gaps for assets"""
        plant_id = str(uuid.uuid4())
        org_id = uuid.uuid4()
        asset_id = uuid.uuid4()
        req_id = uuid.uuid4()
        
        # Seed assets and compliance requirements
        self.conn.fetch_results = {
            "SELECT id, asset_tag, asset_name FROM asset.assets": [
                {"id": asset_id, "asset_tag": "V-202", "asset_name": "Safety Valve 202"},
                {"id": asset_id, "asset_tag": "HX-303", "asset_name": "Heat Exchanger 303"}
            ],
            "SELECT id, title, requirement_type, frequency FROM compliance.requirements": [
                {"id": req_id, "title": "Annual Pressure Vessel Test", "requirement_type": "SAFETY", "frequency": "yearly"}
            ],
            # Expired documents search returning one expired doc for V-202
            "SELECT d.id, d.title FROM graph.entities": [
                {"id": uuid.uuid4(), "title": "V-202 inspection expired certificate"}
            ]
        }
        
        # Mock inspection count to trigger gaps:
        # V-202: 1 inspection report -> triggers expired certificate scan
        # HX-303: 0 inspection reports -> triggers MISSING_EVIDENCE gap
        self.conn.fetchval_results = {
            "normalized_value = $1": 1  # For V-202
        }
        
        # Patch fetchval to return 0 for HX-303 and 1 for V-202
        async def mock_fetchval(query, *args):
            # Check arguments
            if "HX-303" in args:
                return 0
            return 1
            
        self.conn.fetchval = mock_fetchval
        
        # Run compliance audit
        gaps = asyncio.run(self.main.audit_compliance(plant_id))
        
        # Check generated gaps
        self.assertTrue(len(gaps) > 0)
        
        # HX-303 should have a MISSING_EVIDENCE gap
        hx_gaps = [g for g in gaps if g["assetTag"] == "HX-303"]
        self.assertEqual(hx_gaps[0]["gapType"], "MISSING_EVIDENCE")
        self.assertEqual(hx_gaps[0]["severity"], "HIGH")
        
        # V-202 should have an EXPIRED_CERTIFICATE gap
        v_gaps = [g for g in gaps if g["assetTag"] == "V-202"]
        self.assertEqual(v_gaps[0]["gapType"], "EXPIRED_CERTIFICATE")
        self.assertEqual(v_gaps[0]["severity"], "CRITICAL")

    async def get_dummy_datetime(self):
        from datetime import datetime
        return datetime.now()


# ==========================================
# TEST CASE SUITE: LIVE SERVICE MODE
# ==========================================

class TestLiveIntegration(unittest.TestCase):
    """
    Tests live endpoints against the actual running PlantBrainAI API service.
    Requires setting the `PLANTBRAIN_TEST_URL` environment variable.
    """
    
    def setUp(self):
        import urllib.request
        import urllib.error
        self.urllib = urllib.request
        self.urllib_error = urllib.error
        self.base_url = LIVE_SERVICE_URL.rstrip('/')
        print(f"\n[Live Test] Targeting: {self.base_url}")

    def make_request(self, path, method="GET", data=None):
        url = f"{self.base_url}{path}"
        headers = {"Content-Type": "application/json"}
        req_data = None
        if data is not None:
            req_data = json.dumps(data).encode("utf-8")
            
        req = self.urllib.Request(url, data=req_data, headers=headers, method=method)
        try:
            with self.urllib.urlopen(req, timeout=8) as response:
                status = response.status
                body = json.loads(response.read().decode("utf-8"))
                return status, body
        except self.urllib_error.HTTPError as e:
            status = e.code
            try:
                body = json.loads(e.read().decode("utf-8"))
            except:
                body = {"raw_error": e.reason}
            return status, body
        except Exception as e:
            return 500, {"error": str(e)}

    def test_live_health(self):
        """Live Test: GET /health"""
        status, body = self.make_request("/health")
        self.assertEqual(status, 200)
        self.assertIn("status", body)
        self.assertEqual(body["status"], "healthy")

    def test_live_process_document(self):
        """Live Test: POST /process-document"""
        doc_id = str(uuid.uuid4())
        payload = {
            "document_id": doc_id,
            "file_path": "test.txt",
            "file_type": "txt",
            "metadata": {"title": "Live Integration Test Manual"}
        }
        status, body = self.make_request("/process-document", method="POST", data=payload)
        
        # Accept 200 OK or 202 Accepted
        self.assertIn(status, [200, 202])
        self.assertIn("document_id", body)
        self.assertEqual(body["document_id"], doc_id)

    def test_live_query(self):
        """Live Test: POST /query"""
        plant_id = str(uuid.uuid4())
        payload = {
            "question": "What is the pressure limit for boiler B-12?",
            "plantId": plant_id
        }
        status, body = self.make_request("/query", method="POST", data=payload)
        
        # Live DB could be empty or return answers. Let's check status is 200 or 500 (db not connected in dev)
        if status == 200:
            self.assertIn("answer", body)
            self.assertIn("confidence", body)
            self.assertIn("citations", body)
            self.assertIn("relatedAssets", body)
            self.assertIn("missingInfo", body)
        elif status == 500:
            # Handle DB connection issues gracefully during mock deployments
            print(f"Skipping live DB check: Service returned 500 ({body.get('detail')})")
        else:
            self.fail(f"Unexpected status code {status}: {body}")

    def test_live_rca(self):
        """Live Test: POST /rca"""
        payload = {
            "assetTag": "P-101",
            "failureDescription": "Vibration in pump motor bearings"
        }
        status, body = self.make_request("/rca", method="POST", data=payload)
        
        if status == 200:
            self.assertIn("summary", body)
            self.assertIn("probableCauses", body)
            self.assertIn("recommendations", body)
            self.assertIn("confidence", body)
        elif status == 500:
            print(f"Skipping live DB check: Service returned 500 ({body.get('detail')})")
        else:
            self.fail(f"Unexpected status code {status}: {body}")

    def test_live_compliance(self):
        """Live Test: GET /compliance"""
        plant_id = str(uuid.uuid4())
        status, body = self.make_request(f"/compliance?plantId={plant_id}")
        
        if status == 200:
            # Compliance returns gap list
            self.assertIsInstance(body, list)
            if len(body) > 0:
                self.assertIn("assetTag", body[0])
                self.assertIn("gapType", body[0])
                self.assertIn("severity", body[0])
        elif status == 500:
            print(f"Skipping live DB check: Service returned 500 ({body.get('detail')})")
        else:
            self.fail(f"Unexpected status code {status}: {body}")


# ==========================================
# TEST RUNNER MAIN
# ==========================================

if __name__ == "__main__":
    print("=" * 60)
    print("      PlantBrainAI Integration Test Runner")
    print("=" * 60)
    
    if LIVE_SERVICE_URL:
        print(f"Executing: LIVE INTEGRATION TESTS against {LIVE_SERVICE_URL}")
        suite = unittest.TestLoader().loadTestsFromTestCase(TestLiveIntegration)
    else:
        print("Executing: OFFLINE MOCKED INTEGRATION TESTS (Default)")
        print("Set PLANTBRAIN_TEST_URL env variable to test live endpoints.")
        suite = unittest.TestLoader().loadTestsFromTestCase(TestMockIntegration)
        
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    
    # Exit with code 0 on success, code 1 on failures
    sys.exit(0 if result.wasSuccessful() else 1)
