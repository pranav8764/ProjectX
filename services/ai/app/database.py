import asyncio
import asyncpg
from typing import Optional, Any
from psycopg_pool import AsyncConnectionPool
from app.config import DATABASE_URL, logger

db_pool: Optional[asyncpg.Pool] = None
psycopg_pool: Optional[AsyncConnectionPool] = None

async def init_db_pool():
    global db_pool, psycopg_pool
    for i in range(5):
        try:
            db_pool = await asyncpg.create_pool(DATABASE_URL)
            logger.info("Successfully connected to PostgreSQL via asyncpg")
            
            # Also initialize psycopg pool for LangGraph PostgresSaver
            psycopg_pool = AsyncConnectionPool(conninfo=DATABASE_URL, min_size=1, max_size=10)
            await psycopg_pool.open()
            logger.info("Successfully connected to PostgreSQL via psycopg AsyncConnectionPool")
            break
        except Exception as e:
            logger.warning(f"Database connection failed, retrying in 2s ({i+1}/5): {e}")
            await asyncio.sleep(2)
    if not db_pool:
        logger.error("Failed to connect to database. Pool is unavailable.")

async def close_db_pool():
    global db_pool, psycopg_pool
    if db_pool:
        await db_pool.close()
        logger.info("Database connection closed (asyncpg)")
    if psycopg_pool:
        await psycopg_pool.close()
        logger.info("Database connection closed (psycopg)")

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
