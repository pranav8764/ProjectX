-- demo_seed.sql
-- Seed script for PlantBrainAI MVP / Hackathon Demo Dataset
-- Schema-qualified tables: identity, document, ingestion, asset, graph, rca, compliance, rag

BEGIN;

-- Truncate existing data to start with a clean slate
TRUNCATE TABLE
  identity.memberships,
  identity.roles,
  identity.users,
  identity.plants,
  identity.organizations,
  document.document_versions,
  document.documents,
  asset.assets,
  compliance.gaps,
  compliance.requirements,
  rca.reports,
  ingestion.document_chunks,
  ingestion.document_pages,
  graph.entities,
  graph.relationships,
  rag.citations,
  rag.queries
  CASCADE;

DO $$
DECLARE
  v_org_id uuid;
  v_plant_id uuid;
  
  -- Roles
  v_admin_role_id uuid;
  v_eng_role_id uuid;
  v_tech_role_id uuid;
  v_comp_role_id uuid;
  v_pm_role_id uuid;
  
  -- Users
  v_arjun_id uuid;
  v_ravi_id uuid;
  v_meera_id uuid;
  v_suresh_id uuid;
  
  -- Assets
  v_p101_id uuid;
  v_hx204_id uuid;
  v_v301_id uuid;
  v_b12_id uuid;
  
  -- Requirements
  v_req_boiler_id uuid;
  v_req_vibration_id uuid;
  v_req_pressure_id uuid;
  v_req_emissions_id uuid;

  -- Documents
  v_doc_manual_id uuid;
  v_doc_log_id uuid;
  v_doc_wo_id uuid;
  v_doc_ir_id uuid;
  v_doc_sop_id uuid;
  v_doc_safety_id uuid;
  v_doc_audit_id uuid;
  v_doc_incident_id uuid;
  v_doc_compressor_id uuid;
  v_doc_compliance_id uuid;

  -- Document Versions
  v_doc_manual_ver_id uuid;
  v_doc_log_ver_id uuid;
  v_doc_wo_ver_id uuid;
  v_doc_ir_ver_id uuid;
  v_doc_sop_ver_id uuid;
  v_doc_safety_ver_id uuid;
  v_doc_audit_ver_id uuid;
  v_doc_incident_ver_id uuid;
  v_doc_compressor_ver_id uuid;
  v_doc_compliance_ver_id uuid;

  -- Chunk IDs for reference
  v_chunk_log_id uuid;
  v_chunk_wo_id uuid;
  v_chunk_sop_id uuid;
  v_chunk_ir_id uuid;
  v_chunk_inc_id uuid;

  -- Entity IDs
  v_ent_p101_id uuid;
  v_ent_b12_id uuid;
  v_ent_leak_id uuid;
  v_ent_wo223_id uuid;
  v_ent_sop_id uuid;
  v_ent_overheating_id uuid;
BEGIN
  -- 1. Insert Organization
  INSERT INTO identity.organizations (name, industry)
  VALUES ('PlantBrain Corp', 'Manufacturing & Process')
  RETURNING id INTO v_org_id;

  -- 2. Insert Plant
  INSERT INTO identity.plants (organization_id, name, location)
  VALUES (v_org_id, 'Unit-2 Plant', 'Guillermo, Sector 4')
  RETURNING id INTO v_plant_id;

  -- 3. Insert Roles
  INSERT INTO identity.roles (name, description)
  VALUES ('Admin', 'Full administrative access to manage users, documents, and settings.')
  ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
  RETURNING id INTO v_admin_role_id;

  INSERT INTO identity.roles (name, description)
  VALUES ('Engineer', 'Upload documents, run queries, and view asset intelligence/RCA.')
  ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
  RETURNING id INTO v_eng_role_id;

  INSERT INTO identity.roles (name, description)
  VALUES ('Technician', 'View SOPs, checklists, and search asset information.')
  ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
  RETURNING id INTO v_tech_role_id;

  INSERT INTO identity.roles (name, description)
  VALUES ('Compliance Officer', 'View compliance gaps, access standards, and export audit reports.')
  ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
  RETURNING id INTO v_comp_role_id;

  INSERT INTO identity.roles (name, description)
  VALUES ('Plant Manager', 'Access high-level dashboards, operational risk metrics, and reports.')
  ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
  RETURNING id INTO v_pm_role_id;

  -- 4. Insert Users
  INSERT INTO identity.users (organization_id, name, email, email_verified, mobile_no)
  VALUES (v_org_id, 'Arjun', 'arjun@plantbrain.corp', true, '+15550100201')
  RETURNING id INTO v_arjun_id;

  INSERT INTO identity.users (organization_id, name, email, email_verified, mobile_no)
  VALUES (v_org_id, 'Ravi', 'ravi@plantbrain.corp', true, '+15550100202')
  RETURNING id INTO v_ravi_id;

  INSERT INTO identity.users (organization_id, name, email, email_verified, mobile_no)
  VALUES (v_org_id, 'Meera', 'meera@plantbrain.corp', true, '+15550100203')
  RETURNING id INTO v_meera_id;

  INSERT INTO identity.users (organization_id, name, email, email_verified, mobile_no)
  VALUES (v_org_id, 'Suresh', 'suresh@plantbrain.corp', true, '+15550100204')
  RETURNING id INTO v_suresh_id;

  -- 5. Insert Memberships
  INSERT INTO identity.memberships (organization_id, plant_id, user_id, role_id)
  VALUES 
    (v_org_id, v_plant_id, v_arjun_id, v_eng_role_id),
    (v_org_id, v_plant_id, v_ravi_id, v_tech_role_id),
    (v_org_id, v_plant_id, v_meera_id, v_comp_role_id),
    (v_org_id, v_plant_id, v_suresh_id, v_pm_role_id);

  -- 6. Insert demo sessions for authenticated local API access
  INSERT INTO identity.sessions (id, token, user_id, expires_at)
  VALUES
    ('demo-session-arjun', 'dev-token', v_arjun_id, NOW() + INTERVAL '30 days'),
    ('demo-session-meera', 'meera-token', v_meera_id, NOW() + INTERVAL '30 days'),
    ('demo-session-suresh', 'suresh-token', v_suresh_id, NOW() + INTERVAL '30 days'),
    ('demo-session-ravi', 'ravi-token', v_ravi_id, NOW() + INTERVAL '30 days');

  -- 7. Insert Assets (P-101, HX-204, V-301, B-12)
  INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score)
  VALUES (v_org_id, v_plant_id, 'P-101', 'Cooling Water Circulation Pump A', 'Pump', 'Unit-2 Pump House', 'Critical', 78.5)
  RETURNING id INTO v_p101_id;

  INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score)
  VALUES (v_org_id, v_plant_id, 'HX-204', 'Regenerative Heat Exchanger', 'Heat Exchanger', 'Unit-2 Process Hall B', 'High', 42.0)
  RETURNING id INTO v_hx204_id;

  INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score)
  VALUES (v_org_id, v_plant_id, 'V-301', 'High-Pressure Condensate Flash Vessel', 'Vessel', 'Unit-2 Utilities Area', 'Medium', 15.0)
  RETURNING id INTO v_v301_id;

  INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score)
  VALUES (v_org_id, v_plant_id, 'B-12', 'Industrial Auxiliary Steam Boiler', 'Boiler', 'Unit-2 Boiler House', 'Critical', 88.0)
  RETURNING id INTO v_b12_id;

  -- 7. Insert Compliance Requirements
  INSERT INTO compliance.requirements (organization_id, plant_id, title, requirement_type, standard_name, frequency, metadata_json)
  VALUES (v_org_id, v_plant_id, 'Annual Boiler Safety & Integrity Check', 'Inspection', 'PESO Indian Boiler Regulations (IBR)', 'Annual', '{"code": "IBR-1950", "section": "Part III"}')
  RETURNING id INTO v_req_boiler_id;

  INSERT INTO compliance.requirements (organization_id, plant_id, title, requirement_type, standard_name, frequency, metadata_json)
  VALUES (v_org_id, v_plant_id, 'Quarterly Rotating Equipment Alignment & Vibration Audit', 'Audit', 'API 610 / ISO 10816', 'Quarterly', '{"parameters": ["vibration_velocity", "shaft_displacement"]}')
  RETURNING id INTO v_req_vibration_id;

  INSERT INTO compliance.requirements (organization_id, plant_id, title, requirement_type, standard_name, frequency, metadata_json)
  VALUES (v_org_id, v_plant_id, 'Five-Year Pressure Vessel Hydrostatic Pressure Testing', 'Testing', 'ASME Boiler and Pressure Vessel Code Section VIII', '5 Years', '{"test_pressure_factor": 1.3}')
  RETURNING id INTO v_req_pressure_id;

  INSERT INTO compliance.requirements (organization_id, plant_id, title, requirement_type, standard_name, frequency, metadata_json)
  VALUES (v_org_id, v_plant_id, 'Monthly Fugitive Emissions Visual Inspection', 'Inspection', 'EPA Method 21', 'Monthly', '{"monitoring_target": "valves_and_flanges"}')
  RETURNING id INTO v_req_emissions_id;

  -- 8. Insert Compliance Gaps
  INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, requirement_id, gap_type, description, severity, status)
  VALUES (v_org_id, v_plant_id, v_b12_id, v_req_boiler_id, 'MISSING_INSPECTION', 'Annual hydrostatic and safety inspection is overdue by 45 days. Boiler certificate has expired.', 'Critical', 'OPEN');

  INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, requirement_id, gap_type, description, severity, status)
  VALUES (v_org_id, v_plant_id, v_p101_id, v_req_vibration_id, 'MISSING_MAINTENANCE_PROOF', 'Q2 vibration and alignment analysis report is missing for Pump P-101.', 'High', 'OPEN');

  -- 9. Insert Documents (matching the demo requirements of 8-12 documents)
  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Pump P-101 OEM Manual', 'OEM_MANUAL', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_manual_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Pump P-101 Maintenance Log', 'MAINTENANCE_LOG', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_log_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Work Order WO-223', 'WORK_ORDER', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_wo_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Inspection Report IR-91', 'INSPECTION_REPORT', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_ir_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Boiler SOP', 'SOP', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_sop_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Safety Checklist', 'SAFETY_PROCEDURE', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_safety_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Audit Report', 'AUDIT_REPORT', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_audit_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Incident Report', 'INCIDENT_REPORT', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_incident_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Compressor Manual', 'OEM_MANUAL', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_compressor_id;

  INSERT INTO document.documents (organization_id, plant_id, title, document_type, status, uploaded_by)
  VALUES (v_org_id, v_plant_id, 'Compliance Checklist', 'COMPLIANCE_DOCUMENT', 'COMPLETED', v_arjun_id)
  RETURNING id INTO v_doc_compliance_id;

  -- 10. Insert Document Versions
  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_manual_id, 'v1.0', 's3://plantbrain-vault/pump_p-101_oem_manual.pdf', 'application/pdf', md5('Pump P-101 OEM Manual'), 0.99, 0.98)
  RETURNING id INTO v_doc_manual_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_log_id, 'v1.0', 's3://plantbrain-vault/pump_p-101_maintenance_log.pdf', 'application/pdf', md5('Pump P-101 Maintenance Log'), 0.95, 0.92)
  RETURNING id INTO v_doc_log_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_wo_id, 'v1.0', 's3://plantbrain-vault/work_order_wo-223.pdf', 'application/pdf', md5('Work Order WO-223'), 0.97, 0.94)
  RETURNING id INTO v_doc_wo_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_ir_id, 'v1.0', 's3://plantbrain-vault/inspection_report_ir-91.pdf', 'application/pdf', md5('Inspection Report IR-91'), 0.98, 0.96)
  RETURNING id INTO v_doc_ir_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_sop_id, 'v1.0', 's3://plantbrain-vault/boiler_sop.pdf', 'application/pdf', md5('Boiler SOP'), 0.99, 0.99)
  RETURNING id INTO v_doc_sop_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_safety_id, 'v1.0', 's3://plantbrain-vault/safety_checklist.pdf', 'application/pdf', md5('Safety Checklist'), 0.96, 0.95)
  RETURNING id INTO v_doc_safety_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_audit_id, 'v1.0', 's3://plantbrain-vault/audit_report.pdf', 'application/pdf', md5('Audit Report'), 0.98, 0.97)
  RETURNING id INTO v_doc_audit_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_incident_id, 'v1.0', 's3://plantbrain-vault/incident_report.pdf', 'application/pdf', md5('Incident Report'), 0.94, 0.90)
  RETURNING id INTO v_doc_incident_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_compressor_id, 'v1.0', 's3://plantbrain-vault/compressor_manual.pdf', 'application/pdf', md5('Compressor Manual'), 0.99, 0.98)
  RETURNING id INTO v_doc_compressor_ver_id;

  INSERT INTO document.document_versions (document_id, version_label, file_url, file_type, file_sha256, ocr_confidence, classification_confidence)
  VALUES (v_doc_compliance_id, 'v1.0', 's3://plantbrain-vault/compliance_checklist.pdf', 'application/pdf', md5('Compliance Checklist'), 0.98, 0.96)
  RETURNING id INTO v_doc_compliance_ver_id;

  -- Update Documents current_version_id
  UPDATE document.documents SET current_version_id = v_doc_manual_ver_id WHERE id = v_doc_manual_id;
  UPDATE document.documents SET current_version_id = v_doc_log_ver_id WHERE id = v_doc_log_id;
  UPDATE document.documents SET current_version_id = v_doc_wo_ver_id WHERE id = v_doc_wo_id;
  UPDATE document.documents SET current_version_id = v_doc_ir_ver_id WHERE id = v_doc_ir_id;
  UPDATE document.documents SET current_version_id = v_doc_sop_ver_id WHERE id = v_doc_sop_id;
  UPDATE document.documents SET current_version_id = v_doc_safety_ver_id WHERE id = v_doc_safety_id;
  UPDATE document.documents SET current_version_id = v_doc_audit_ver_id WHERE id = v_doc_audit_id;
  UPDATE document.documents SET current_version_id = v_doc_incident_ver_id WHERE id = v_doc_incident_id;
  UPDATE document.documents SET current_version_id = v_doc_compressor_ver_id WHERE id = v_doc_compressor_id;
  UPDATE document.documents SET current_version_id = v_doc_compliance_ver_id WHERE id = v_doc_compliance_id;

  -- 11. Insert Document Pages
  INSERT INTO ingestion.document_pages (document_id, document_version_id, page_no, raw_text, markdown_text, ocr_confidence, metadata_json)
  VALUES (v_doc_log_id, v_doc_log_ver_id, 1, 
          'Annual maintenance log for Unit-2 Cooling Water Pump P-101. On 12 Jan 2025, technician Ravi reported bearing seal leakage and high vibration velocity. Bearing housing was inspected.',
          '# Pump P-101 Maintenance Log\nAnnual maintenance log for Unit-2 Cooling Water Pump P-101.\n\nOn 12 Jan 2025, technician Ravi reported bearing seal leakage and high vibration velocity. Bearing housing was inspected.',
          0.95, '{"asset_tags": ["P-101"]}');

  INSERT INTO ingestion.document_pages (document_id, document_version_id, page_no, raw_text, markdown_text, ocr_confidence, metadata_json)
  VALUES (v_doc_wo_id, v_doc_wo_ver_id, 1,
          'Work Order WO-223. Asset: P-101. Date: 14 Jan 2025. Description: Replace primary mechanical seal and outer bearings due to heavy leakage. Recommended action: perform quarterly shaft alignment.',
          '# Work Order WO-223\n* **Asset:** P-101\n* **Date:** 14 Jan 2025\n\n**Description:** Replace primary mechanical seal and outer bearings due to heavy leakage.\n\n**Recommended action:** perform quarterly shaft alignment.',
          0.97, '{"asset_tags": ["P-101"], "work_order": "WO-223"}');

  INSERT INTO ingestion.document_pages (document_id, document_version_id, page_no, raw_text, markdown_text, ocr_confidence, metadata_json)
  VALUES (v_doc_sop_id, v_doc_sop_ver_id, 1,
          'Standard Operating Procedure (SOP) for Industrial Boiler B-12 Startup. Author: Arjun. Ensure water levels are optimal. Slowly open main steam valve. Set burner pressure to 12.5 bar.',
          '# Boiler B-12 Startup SOP\n* **Author:** Arjun\n\n1. Ensure water levels are optimal.\n2. Slowly open main steam valve.\n3. Set burner pressure to 12.5 bar.',
          0.99, '{"asset_tags": ["B-12"], "doc_type": "SOP"}');

  INSERT INTO ingestion.document_pages (document_id, document_version_id, page_no, raw_text, markdown_text, ocr_confidence, metadata_json)
  VALUES (v_doc_ir_id, v_doc_ir_ver_id, 1,
          'Inspection Report IR-91. Asset Tag: B-12. Inspected on 01 Jun 2025. Results: Found minor scale build-up inside the safety valve. Hydrostatic test was not completed due to scheduling conflict.',
          '# Inspection Report IR-91\n* **Asset Tag:** B-12\n* **Date:** 01 Jun 2025\n\n**Results:** Found minor scale build-up inside the safety valve.\n\n**Note:** Hydrostatic test was not completed due to scheduling conflict.',
          0.98, '{"asset_tags": ["B-12"], "report_id": "IR-91"}');

  INSERT INTO ingestion.document_pages (document_id, document_version_id, page_no, raw_text, markdown_text, ocr_confidence, metadata_json)
  VALUES (v_doc_incident_id, v_doc_incident_ver_id, 1,
          'Incident Report. Asset: P-101. Date: 20 Feb 2025. Maintenance Team reported sudden failure of seal on Pump P-101 due to running dry during startup. Cause: missing SOP adherence.',
          '# Incident Report\n* **Asset:** P-101\n* **Date:** 20 Feb 2025\n\n**Event:** Maintenance Team reported sudden failure of seal on Pump P-101 due to running dry during startup.\n\n**Root Cause:** Missing SOP adherence.',
          0.94, '{"asset_tags": ["P-101"]}');

  -- 12. Insert Document Chunks with 1536-dimensional vectors
  INSERT INTO ingestion.document_chunks (document_id, document_version_id, page_no, chunk_index, chunk_text, embedding, token_count, metadata_json)
  VALUES (v_doc_log_id, v_doc_log_ver_id, 1, 0, 
          'Annual maintenance log for Unit-2 Cooling Water Pump P-101. On 12 Jan 2025, technician Ravi reported bearing seal leakage and high vibration velocity. Bearing housing was inspected.',
          array_fill(0.0::double precision, ARRAY[1536])::vector, 34, '{"asset_tags": ["P-101"]}')
  RETURNING id INTO v_chunk_log_id;

  INSERT INTO ingestion.document_chunks (document_id, document_version_id, page_no, chunk_index, chunk_text, embedding, token_count, metadata_json)
  VALUES (v_doc_wo_id, v_doc_wo_ver_id, 1, 0,
          'Work Order WO-223. Asset: P-101. Date: 14 Jan 2025. Description: Replace primary mechanical seal and outer bearings due to heavy leakage. Recommended action: perform quarterly shaft alignment.',
          array_fill(0.0::double precision, ARRAY[1536])::vector, 34, '{"asset_tags": ["P-101"], "work_order": "WO-223"}')
  RETURNING id INTO v_chunk_wo_id;

  INSERT INTO ingestion.document_chunks (document_id, document_version_id, page_no, chunk_index, chunk_text, embedding, token_count, metadata_json)
  VALUES (v_doc_sop_id, v_doc_sop_ver_id, 1, 0,
          'Standard Operating Procedure (SOP) for Industrial Boiler B-12 Startup. Author: Arjun. Ensure water levels are optimal. Slowly open main steam valve. Set burner pressure to 12.5 bar.',
          array_fill(0.0::double precision, ARRAY[1536])::vector, 32, '{"asset_tags": ["B-12"], "doc_type": "SOP"}')
  RETURNING id INTO v_chunk_sop_id;

  INSERT INTO ingestion.document_chunks (document_id, document_version_id, page_no, chunk_index, chunk_text, embedding, token_count, metadata_json)
  VALUES (v_doc_ir_id, v_doc_ir_ver_id, 1, 0,
          'Inspection Report IR-91. Asset Tag: B-12. Inspected on 01 Jun 2025. Results: Found minor scale build-up inside the safety valve. Hydrostatic test was not completed due to scheduling conflict.',
          array_fill(0.0::double precision, ARRAY[1536])::vector, 34, '{"asset_tags": ["B-12"], "report_id": "IR-91"}')
  RETURNING id INTO v_chunk_ir_id;

  INSERT INTO ingestion.document_chunks (document_id, document_version_id, page_no, chunk_index, chunk_text, embedding, token_count, metadata_json)
  VALUES (v_doc_incident_id, v_doc_incident_ver_id, 1, 0,
          'Incident Report. Asset: P-101. Date: 20 Feb 2025. Maintenance Team reported sudden failure of seal on Pump P-101 due to running dry during startup. Cause: missing SOP adherence.',
          array_fill(0.0::double precision, ARRAY[1536])::vector, 33, '{"asset_tags": ["P-101"]}')
  RETURNING id INTO v_chunk_inc_id;

  -- 13. Insert Graph Entities
  INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
  VALUES (v_org_id, v_plant_id, v_doc_log_id, v_chunk_log_id, 'Asset', 'Pump P-101', 'P-101', 0.98, 1)
  RETURNING id INTO v_ent_p101_id;

  INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
  VALUES (v_org_id, v_plant_id, v_doc_sop_id, v_chunk_sop_id, 'Asset', 'Boiler B-12', 'B-12', 0.99, 1)
  RETURNING id INTO v_ent_b12_id;

  INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
  VALUES (v_org_id, v_plant_id, v_doc_log_id, v_chunk_log_id, 'Failure', 'Bearing Seal Leakage', 'seal_leakage', 0.92, 1)
  RETURNING id INTO v_ent_leak_id;

  INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
  VALUES (v_org_id, v_plant_id, v_doc_wo_id, v_chunk_wo_id, 'WorkOrder', 'WO-223', 'WO-223', 0.95, 1)
  RETURNING id INTO v_ent_wo223_id;

  INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
  VALUES (v_org_id, v_plant_id, v_doc_sop_id, v_chunk_sop_id, 'SOP', 'Boiler Startup Procedure', 'boiler_startup_sop', 0.97, 1)
  RETURNING id INTO v_ent_sop_id;

  INSERT INTO graph.entities (organization_id, plant_id, document_id, chunk_id, entity_type, entity_value, normalized_value, confidence, page_no)
  VALUES (v_org_id, v_plant_id, v_doc_log_id, v_chunk_log_id, 'Failure', 'High Vibration Velocity', 'high_vibration', 0.90, 1)
  RETURNING id INTO v_ent_overheating_id;

  -- 14. Insert Graph Relationships
  INSERT INTO graph.relationships (organization_id, source_entity_id, target_entity_id, relationship_type, confidence, evidence_document_id, evidence_chunk_id)
  VALUES 
    (v_org_id, v_ent_p101_id, v_ent_leak_id, 'HAS_FAILURE', 0.95, v_doc_log_id, v_chunk_log_id),
    (v_org_id, v_ent_p101_id, v_ent_overheating_id, 'HAS_FAILURE', 0.93, v_doc_log_id, v_chunk_log_id),
    (v_org_id, v_ent_leak_id, v_ent_wo223_id, 'RESOLVED_BY', 0.90, v_doc_wo_id, v_chunk_wo_id),
    (v_org_id, v_ent_b12_id, v_ent_sop_id, 'REQUIRES_SOP', 0.98, v_doc_sop_id, v_chunk_sop_id);

  -- 15. Insert RCA Reports
  INSERT INTO rca.reports (organization_id, plant_id, asset_id, failure_summary, timeline, probable_causes, recommendations, missing_data, confidence, created_by)
  VALUES (
    v_org_id, 
    v_plant_id, 
    v_p101_id, 
    'Pump P-101 has suffered repeated mechanical seal leakage and outer bearing overheating leading to sudden failure.',
    '[
       {"timestamp": "2025-01-12T08:00:00Z", "event": "Technician Ravi reported bearing seal leakage and high vibration velocity on P-101"},
       {"timestamp": "2025-01-14T10:00:00Z", "event": "Work Order WO-223 executed. Seal and bearings replaced by maintenance team"},
       {"timestamp": "2025-02-20T14:30:00Z", "event": "Pump P-101 experienced sudden seal blowout. Incident report filed"}
     ]'::jsonb,
    '[
       {"cause": "Shaft misalignment causing high radial vibration leading to seal deterioration", "probability": 0.65},
       {"cause": "Dry running of pump during startup before filling casing", "probability": 0.35}
     ]'::jsonb,
    '[
       {"action": "Enforce strict checklist compliance for pump startup sequence to prevent dry running", "priority": "High"},
       {"action": "Perform quarterly laser alignment audits on all critical rotating assets", "priority": "High"}
     ]'::jsonb,
    '["Post-maintenance vibration signature logs for late January", "Operator pre-start check log for February 20"]'::jsonb,
    0.85,
    v_suresh_id
  );

  -- 16. Insert sample query/citations for testing
  DECLARE
    v_query_id uuid;
  BEGIN
    INSERT INTO rag.queries (organization_id, plant_id, user_id, query_text, answer_text, confidence)
    VALUES (
      v_org_id,
      v_plant_id,
      v_arjun_id,
      'Why does Pump P-101 keep failing?',
      'Pump P-101 has had recurring failures primarily due to mechanical seal leakage and high vibration, as documented in the Maintenance Log (12 Jan 2025) and Work Order WO-223 (14 Jan 2025). The root causes include shaft misalignment and dry running.',
      0.88
    ) RETURNING id INTO v_query_id;

    INSERT INTO rag.citations (query_id, document_id, chunk_id, page_no, quoted_text, score)
    VALUES 
      (v_query_id, v_doc_log_id, v_chunk_log_id, 1, 'technician Ravi reported bearing seal leakage and high vibration velocity', 0.92),
      (v_query_id, v_doc_wo_id, v_chunk_wo_id, 1, 'Replace primary mechanical seal and outer bearings due to heavy leakage', 0.95);
  END;

END $$;

COMMIT;
