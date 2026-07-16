#!/usr/bin/env python3
"""
PlantBrainAI AI-service unit tests (dependency-light, deterministic).

Runs with no third-party packages installed: heavy/external modules (spacy, google
genai, groq, asyncpg, pgvector, langgraph, pdf libs) are stubbed in sys.modules before
importing the app, so this suite exercises the *pure* logic that the ingestion/query
fixes touch — entity regexes, asset-tag normalization, citation validation, date-filter
parsing. Full graph/RAG behavior is covered by tests/integration_test.py in live mode
(PLANTBRAIN_TEST_URL) against a running service.

    python3 tests/unit_ai_test.py
"""

import os
import re
import sys
import types
import unittest
from unittest.mock import MagicMock

WORKSPACE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(WORKSPACE, "services", "ai"))


def _install_module(name, module):
    sys.modules.setdefault(name, module)


def _stub_heavy_dependencies():
    """Insert lightweight stand-ins so `import app.*` succeeds without real packages."""
    # fastapi: routers only need to accept decorator registrations at import time
    fastapi_mod = types.ModuleType("fastapi")

    class _APIRouter:
        def _decorator(self, *_a, **_k):
            def wrap(fn):
                return fn
            return wrap

        post = _decorator
        get = _decorator
        put = _decorator
        delete = _decorator

    class _HTTPException(Exception):
        def __init__(self, status_code=None, detail=None, *a, **k):
            super().__init__(detail or status_code)
            self.status_code = status_code
            self.detail = detail

    fastapi_mod.APIRouter = _APIRouter
    fastapi_mod.BackgroundTasks = MagicMock
    fastapi_mod.HTTPException = _HTTPException
    fastapi_mod.Request = MagicMock
    _install_module("fastapi", fastapi_mod)

    # pydantic: schemas subclass BaseModel and use class-level annotations only
    pydantic_mod = types.ModuleType("pydantic")

    class _BaseModel:
        def __init__(self, **kwargs):
            for k, v in kwargs.items():
                setattr(self, k, v)

    pydantic_mod.BaseModel = _BaseModel
    _install_module("pydantic", pydantic_mod)

    _install_module("numpy", MagicMock())

    # typing_extensions may be absent on a bare interpreter; back it with stdlib typing
    try:
        import typing_extensions  # noqa: F401
    except ImportError:
        import typing as _typing
        te = types.ModuleType("typing_extensions")
        te.TypedDict = _typing.TypedDict
        te.Annotated = _typing.Annotated
        _install_module("typing_extensions", te)

    # spacy: config.py calls spacy.load(...) at import time and expects a callable nlp
    # returning an object with `.ents`. Return no entities so regex behavior is isolated.
    spacy_mod = types.ModuleType("spacy")

    def _load(*_args, **_kwargs):
        def _nlp(_text):
            return types.SimpleNamespace(ents=[])
        return _nlp

    spacy_mod.load = _load
    _install_module("spacy", spacy_mod)

    # google.genai / groq clients — only instantiated when API keys are set (they are not)
    google_mod = types.ModuleType("google")
    genai_mod = types.ModuleType("google.genai")
    genai_mod.Client = MagicMock()
    types_mod = types.ModuleType("google.genai.types")
    types_mod.EmbedContentConfig = MagicMock()
    genai_mod.types = types_mod
    google_mod.genai = genai_mod
    _install_module("google", google_mod)
    _install_module("google.genai", genai_mod)
    _install_module("google.genai.types", types_mod)

    groq_mod = types.ModuleType("groq")
    groq_mod.Groq = MagicMock()
    _install_module("groq", groq_mod)

    # DB / vector stack used by app.database
    _install_module("asyncpg", MagicMock())
    pgvector_mod = types.ModuleType("pgvector")
    pgvector_asyncpg = types.ModuleType("pgvector.asyncpg")
    pgvector_asyncpg.register_vector = MagicMock()
    pgvector_mod.asyncpg = pgvector_asyncpg
    _install_module("pgvector", pgvector_mod)
    _install_module("pgvector.asyncpg", pgvector_asyncpg)
    psycopg_pool = types.ModuleType("psycopg_pool")
    psycopg_pool.AsyncConnectionPool = MagicMock()
    _install_module("psycopg_pool", psycopg_pool)

    # Document-processing libs imported by app.routes.ingestion
    for name in ("PyPDF2", "pdfplumber", "pandas", "pytesseract", "pdf2image"):
        _install_module(name, MagicMock())
    docx_mod = types.ModuleType("docx")
    docx_mod.Document = MagicMock()
    _install_module("docx", docx_mod)
    pil_mod = types.ModuleType("PIL")
    pil_mod.Image = MagicMock()
    _install_module("PIL", pil_mod)

    # langgraph / langchain, imported by app.graph (only names need to resolve at import)
    lg_graph = types.ModuleType("langgraph.graph")
    lg_graph.StateGraph = MagicMock()
    lg_graph.START = "START"
    lg_graph.END = "END"
    lg_root = types.ModuleType("langgraph")
    lg_root.graph = lg_graph
    lg_msg = types.ModuleType("langgraph.graph.message")
    lg_msg.add_messages = MagicMock()
    lg_ckpt = types.ModuleType("langgraph.checkpoint.memory")
    lg_ckpt.MemorySaver = MagicMock()
    _install_module("langgraph", lg_root)
    _install_module("langgraph.graph", lg_graph)
    _install_module("langgraph.graph.message", lg_msg)
    _install_module("langgraph.checkpoint.memory", lg_ckpt)
    lc_msg = types.ModuleType("langchain_core.messages")
    lc_msg.HumanMessage = MagicMock()
    lc_root = types.ModuleType("langchain_core")
    lc_root.messages = lc_msg
    _install_module("langchain_core", lc_root)
    _install_module("langchain_core.messages", lc_msg)


_stub_heavy_dependencies()

from app.utils.helpers import (  # noqa: E402
    normalize_asset_tag,
    extract_asset_tags,
    answer_has_citation,
)
from app.routes.ingestion import extract_entities  # noqa: E402
from app.graph import parse_filter_datetime  # noqa: E402


class TestAssetTagNormalization(unittest.TestCase):
    def test_whitespace_and_case_collapse_to_canonical(self):
        # The bug this guards: "P 101" and "P-101" produced duplicate assets.
        self.assertEqual(normalize_asset_tag("P 101"), "P-101")
        self.assertEqual(normalize_asset_tag("p-101"), "P-101")
        self.assertEqual(normalize_asset_tag("  P  101 "), "P-101")
        self.assertEqual(normalize_asset_tag("HX_204"), "HX-204")
        self.assertEqual(
            normalize_asset_tag("P 101"), normalize_asset_tag("P-101")
        )

    def test_extract_asset_tags_normalizes(self):
        tags = extract_asset_tags("Inspect pump P 101 and valve V-301.")
        self.assertIn("P-101", tags)
        self.assertIn("V-301", tags)


class TestCitationValidation(unittest.TestCase):
    def test_requires_bracketed_index_within_range(self):
        self.assertTrue(answer_has_citation("Seal failed [1].", 2))
        self.assertFalse(answer_has_citation("No citation here.", 2))
        self.assertFalse(answer_has_citation("Out of range [5].", 2))
        self.assertFalse(answer_has_citation("Anything [1].", 0))


class TestDateFilterParsing(unittest.TestCase):
    def test_iso_strings_become_tz_aware_datetimes(self):
        d = parse_filter_datetime("2025-03-01")
        self.assertIsNotNone(d)
        self.assertIsNotNone(d.tzinfo)
        d2 = parse_filter_datetime("2025-03-01T12:00:00Z")
        self.assertIsNotNone(d2.tzinfo)

    def test_empty_and_bad_values_return_none(self):
        self.assertIsNone(parse_filter_datetime(None))
        self.assertIsNone(parse_filter_datetime(""))
        self.assertIsNone(parse_filter_datetime("not-a-date"))


class TestEntityExtractionRegexes(unittest.TestCase):
    def _types(self, text):
        ents = extract_entities([{"chunk_index": 0, "text": text, "page_no": 1}])
        return ents

    def test_dates_do_not_span_across_text(self):
        # The old regex had an unclosed character class that matched giant spans.
        text = "Logged 12/01/2025 and 12-01-25 and 12 Jan 2025 for the pump."
        dates = [e["entity_text"] for e in self._types(text) if e["entity_type"] == "DATE"]
        self.assertIn("12/01/2025", dates)
        self.assertIn("12 Jan 2025", dates)
        # No single DATE entity should swallow the whole sentence.
        self.assertTrue(all(len(d) <= 15 for d in dates), dates)

    def test_severity_only_in_labeled_context(self):
        noisy = "The oil level was low and the tank was high."
        sev = [e["entity_text"] for e in self._types(noisy) if e["entity_type"] == "SEVERITY"]
        self.assertEqual(sev, [], f"bare low/high should not be flagged: {sev}")

        labeled = "Severity: High. This is a critical fault."
        sev2 = [e["entity_type"] for e in self._types(labeled) if e["entity_type"] == "SEVERITY"]
        self.assertTrue(len(sev2) >= 1)

    def test_equipment_tag_detected(self):
        tags = [e["entity_text"] for e in self._types("Pump P-101 tripped.")
                if e["entity_type"] == "EQUIPMENT_TAG"]
        self.assertIn("P-101", tags)


if __name__ == "__main__":
    unittest.main(verbosity=2)
