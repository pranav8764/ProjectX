import asyncio
import asyncpg
from typing import Optional, Any
from app.config import DATABASE_URL, logger

db_pool: Optional[asyncpg.Pool] = None

async def init_db_pool():
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
        logger.error("Failed to connect to database. Pool is unavailable.")

async def close_db_pool():
    global db_pool
    if db_pool:
        await db_pool.close()
        logger.info("Database connection closed")

async def get_document_access_columns(conn: Any) -> set:
    try:
        rows = await conn.fetch("""
            SELECT column_name
            FROM information_schema.columns
            WHERE table_schema = 'document'
              AND table_name = 'documents'
              AND column_name IN ('access_level', 'allowed_roles')
        """)
        # We define a helper row_get here to prevent circular dependencies, or import it.
        # But let's define a simple row lookup.
        return {row["column_name"] for row in rows if row["column_name"]}
    except Exception as e:
        logger.warning(f"Unable to inspect document access columns; defaulting to legacy-compatible retrieval: {e}")
        return set()

def document_access_select(access_columns: set) -> str:
    access_expr = "d.access_level" if "access_level" in access_columns else "NULL"
    roles_expr = "d.allowed_roles" if "allowed_roles" in access_columns else "NULL"
    return f"{access_expr} AS access_level, {roles_expr} AS allowed_roles"
