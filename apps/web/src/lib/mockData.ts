export interface Document {
  id: string;
  title: string;
  fileType: string;
  documentType: string;
  status: 'UPLOADED' | 'EXTRACTING_TEXT' | 'OCR_RUNNING' | 'CONVERTING_TO_MARKDOWN' | 'STORING_PAGES' | 'DETECTING_TABLES' | 'CLASSIFYING' | 'CHUNKING' | 'EXTRACTING_ENTITIES' | 'GENERATING_EMBEDDINGS' | 'BUILDING_GRAPH' | 'STORING_RESULTS' | 'COMPLETED' | 'FAILED' | 'PARTIAL_SUCCESS';
  ocrConfidence: number;
  classificationConfidence: number;
  createdAt: string;
  size: string;
  plantName?: string;
  department?: string;
  version?: string;
  assetTags?: string[];
  uploadedBy?: string;
  accessLevel?: string | null;
  sensitivity?: string | null;
  allowedRoles?: string[] | null;
  sourceRestricted?: boolean | null;
  sourceDownloadAllowed?: boolean | null;
}

export interface FailureEvent {
  id: string;
  date: string;
  description: string;
  severity: 'Low' | 'Medium' | 'High' | 'Critical';
  status: 'Open' | 'Resolved';
  workOrder: string;
  maintenanceAction: string;
}

export interface Asset {
  id: string;
  assetTag: string;
  assetName: string;
  assetType: string;
  location: string;
  criticality: 'Critical' | 'High' | 'Medium' | 'Low';
  riskScore: number;
  failures: FailureEvent[];
  documents: string[]; // Document IDs
  complianceGaps: string[]; // Gap IDs
}

export interface ComplianceGap {
  id: string;
  assetTag: string;
  gapType: string;
  description: string;
  severity: 'Critical' | 'High' | 'Medium' | 'Low';
  evidenceDocId?: string;
  status: 'Open' | 'Closed';
  createdAt: string;
}

export interface Certificate {
  id: string;
  name: string;
  assetTag: string;
  expiryDate: string;
  status: 'Active' | 'Expired' | 'Overdue';
}

export const INITIAL_DOCUMENTS: Document[] = [
  {
    id: 'doc_101',
    title: 'Pump P-101 OEM Manual.pdf',
    fileType: 'PDF',
    documentType: 'OEM Manual',
    status: 'COMPLETED',
    ocrConfidence: 0.98,
    classificationConfidence: 0.95,
    createdAt: '2026-06-15T09:30:00Z',
    size: '14.2 MB'
  },
  {
    id: 'doc_102',
    title: 'Pump P-101 Maintenance Log 2025.xlsx',
    fileType: 'XLSX',
    documentType: 'Maintenance Work Order',
    status: 'COMPLETED',
    ocrConfidence: 0.99,
    classificationConfidence: 0.92,
    createdAt: '2026-06-18T14:15:00Z',
    size: '2.4 MB'
  },
  {
    id: 'doc_103',
    title: 'Work Order WO-223 Seal Replacement.pdf',
    fileType: 'PDF',
    documentType: 'Maintenance Work Order',
    status: 'COMPLETED',
    ocrConfidence: 0.88,
    classificationConfidence: 0.97,
    createdAt: '2026-06-20T11:00:00Z',
    size: '850 KB'
  },
  {
    id: 'doc_104',
    title: 'Inspection Report IR-91 Pump Vibration.pdf',
    fileType: 'PDF',
    documentType: 'Inspection Report',
    status: 'COMPLETED',
    ocrConfidence: 0.91,
    classificationConfidence: 0.94,
    createdAt: '2026-06-22T16:45:00Z',
    size: '1.2 MB'
  },
  {
    id: 'doc_105',
    title: 'SOP-BOI-04 Boiler B-12 Startup.pdf',
    fileType: 'PDF',
    documentType: 'SOP',
    status: 'COMPLETED',
    ocrConfidence: 0.99,
    classificationConfidence: 0.99,
    createdAt: '2026-06-25T08:00:00Z',
    size: '3.1 MB'
  },
  {
    id: 'doc_106',
    title: 'Safety Checklist Hot Work v2.pdf',
    fileType: 'PDF',
    documentType: 'Safety Procedure',
    status: 'COMPLETED',
    ocrConfidence: 0.95,
    classificationConfidence: 0.91,
    createdAt: '2026-06-26T10:30:00Z',
    size: '420 KB'
  },
  {
    id: 'doc_107',
    title: 'Q3 Internal Compliance Audit.pdf',
    fileType: 'PDF',
    documentType: 'Audit Report',
    status: 'COMPLETED',
    ocrConfidence: 0.89,
    classificationConfidence: 0.96,
    createdAt: '2026-06-28T13:20:00Z',
    size: '5.6 MB'
  },
  {
    id: 'doc_108',
    title: 'Incident Report IR-2026-04 Leakage.pdf',
    fileType: 'PDF',
    documentType: 'Incident Report',
    status: 'COMPLETED',
    ocrConfidence: 0.94,
    classificationConfidence: 0.90,
    createdAt: '2026-06-29T15:10:00Z',
    size: '1.1 MB'
  },
  {
    id: 'doc_109',
    title: 'Compressor C-204 Technical Specifications.pdf',
    fileType: 'PDF',
    documentType: 'OEM Manual',
    status: 'COMPLETED',
    ocrConfidence: 0.97,
    classificationConfidence: 0.96,
    createdAt: '2026-06-30T09:00:00Z',
    size: '8.4 MB'
  },
  {
    id: 'doc_110',
    title: 'Plant Compliance Guidelines.pdf',
    fileType: 'PDF',
    documentType: 'Compliance Document',
    status: 'COMPLETED',
    ocrConfidence: 0.96,
    classificationConfidence: 0.98,
    createdAt: '2026-07-01T11:45:00Z',
    size: '12.1 MB'
  }
];

export const INITIAL_ASSETS: Asset[] = [
  {
    id: 'asset_1',
    assetTag: 'P-101',
    assetName: 'Main Water Feed Pump',
    assetType: 'Pump',
    location: 'Unit-2 Primary Feed',
    criticality: 'Critical',
    riskScore: 78,
    failures: [
      {
        id: 'fail_101',
        date: '2026-03-12',
        description: 'Primary seal mechanical leakage with minor shaft scoring',
        severity: 'High',
        status: 'Resolved',
        workOrder: 'WO-223',
        maintenanceAction: 'Mechanical seal replaced, shaft polished, aligned motor'
      },
      {
        id: 'fail_102',
        date: '2026-05-04',
        description: 'High frequency vibrations in outboard bearing casing',
        severity: 'Medium',
        status: 'Resolved',
        workOrder: 'WO-281',
        maintenanceAction: 'Lubricated bearing housing, adjusted mounting bolts'
      },
      {
        id: 'fail_103',
        date: '2026-06-28',
        description: 'Repeated seal leakage and pressure drop across intake chamber',
        severity: 'Critical',
        status: 'Open',
        workOrder: 'WO-331',
        maintenanceAction: 'Awaiting diagnosis / RCA recommendations'
      }
    ],
    documents: ['doc_101', 'doc_102', 'doc_103', 'doc_104', 'doc_108'],
    complianceGaps: ['gap_201']
  },
  {
    id: 'asset_2',
    assetTag: 'C-204',
    assetName: 'High Pressure Air Compressor',
    assetType: 'Compressor',
    location: 'Utility Block A',
    criticality: 'High',
    riskScore: 42,
    failures: [
      {
        id: 'fail_201',
        date: '2026-02-18',
        description: 'Overheating in stage-2 discharge valve',
        severity: 'High',
        status: 'Resolved',
        workOrder: 'WO-198',
        maintenanceAction: 'Cleaned discharge valve assembly, replaced gaskets'
      }
    ],
    documents: ['doc_109'],
    complianceGaps: ['gap_203']
  },
  {
    id: 'asset_3',
    assetTag: 'B-12',
    assetName: 'Superheated Steam Boiler',
    assetType: 'Boiler',
    location: 'Boiler House East',
    criticality: 'Critical',
    riskScore: 65,
    failures: [
      {
        id: 'fail_301',
        date: '2026-04-01',
        description: 'Feedwater preheater tubes scale buildup causing low transfer efficiency',
        severity: 'Medium',
        status: 'Resolved',
        workOrder: 'WO-242',
        maintenanceAction: 'Acid washed preheater tubes, flushed feed line'
      }
    ],
    documents: ['doc_105', 'doc_106'],
    complianceGaps: ['gap_202']
  },
  {
    id: 'asset_4',
    assetTag: 'HX-204',
    assetName: 'Shell & Tube Heat Exchanger',
    assetType: 'Heat Exchanger',
    location: 'Unit-3 Condensation Loop',
    criticality: 'Medium',
    riskScore: 18,
    failures: [],
    documents: [],
    complianceGaps: []
  },
  {
    id: 'asset_5',
    assetTag: 'V-301',
    assetName: 'Emergency Shutdown Solenoid Valve',
    assetType: 'Valve',
    location: 'Unit-2 Gas Header',
    criticality: 'Critical',
    riskScore: 55,
    failures: [
      {
        id: 'fail_501',
        date: '2026-01-10',
        description: 'Solenoid coil electrical burnout',
        severity: 'High',
        status: 'Resolved',
        workOrder: 'WO-120',
        maintenanceAction: 'Replaced coil assembly, verified safety interlock loop'
      }
    ],
    documents: ['doc_107'],
    complianceGaps: ['gap_204']
  }
];

export const INITIAL_COMPLIANCE_GAPS: ComplianceGap[] = [
  {
    id: 'gap_201',
    assetTag: 'P-101',
    gapType: 'Missing Vibration Report',
    description: 'Post-seal installation vibration analysis report is missing in documentation history after March 2025.',
    severity: 'High',
    status: 'Open',
    createdAt: '2026-06-22T16:45:00Z'
  },
  {
    id: 'gap_202',
    assetTag: 'B-12',
    gapType: 'Expired Hydrostatic Certificate',
    description: 'Annual hydrostatic pressure test certificate expired on May 30, 2026. Code B-12 rules require active certificate for operation.',
    severity: 'Critical',
    status: 'Open',
    createdAt: '2026-05-31T00:01:00Z'
  },
  {
    id: 'gap_203',
    assetTag: 'C-204',
    gapType: 'Missing SOP Training Logs',
    description: 'SOP training evidence not updated for newly deployed operators working on C-204.',
    severity: 'Medium',
    status: 'Open',
    createdAt: '2026-06-30T09:00:00Z'
  },
  {
    id: 'gap_204',
    assetTag: 'V-301',
    gapType: 'Safety Valve Cal Overdue',
    description: 'Biannual safety valve calibration inspection overdue by 24 days.',
    severity: 'High',
    status: 'Open',
    createdAt: '2026-06-09T00:00:00Z'
  }
];

export const INITIAL_CERTIFICATES: Certificate[] = [
  {
    id: 'cert_1',
    name: 'Hydrostatic Testing Certificate',
    assetTag: 'B-12',
    expiryDate: '2026-05-30',
    status: 'Expired'
  },
  {
    id: 'cert_2',
    name: 'Safety Interlock Loop Verification',
    assetTag: 'V-301',
    expiryDate: '2026-12-15',
    status: 'Active'
  },
  {
    id: 'cert_3',
    name: 'Thickness Measurement Audit',
    assetTag: 'HX-204',
    expiryDate: '2026-06-01',
    status: 'Overdue'
  },
  {
    id: 'cert_4',
    name: 'Electrical Safety Grounding Pass',
    assetTag: 'P-101',
    expiryDate: '2027-04-10',
    status: 'Active'
  }
];

export interface CopilotResponse {
  answer: string;
  confidence: number;
  citations: Array<{
    documentTitle: string;
    page: number;
    snippet: string;
  }>;
  relatedAssets: string[];
  missingInfo: string[];
}

export const queryCopilot = (question: string, assetTagFilter?: string): CopilotResponse => {
  const q = question.toLowerCase();
  
  if (q.includes('p-101') || q.includes('pump') && (q.includes('fail') || q.includes('history') || q.includes('leak'))) {
    return {
      answer: "Based on the maintenance logs and incident reports for Pump P-101, the asset has experienced recurrent shaft seal leakage. The most critical event was logged on March 12, 2026 (WO-223), where mechanical seal failure led to minor shaft scoring. The latest logged event on June 28, 2026 (WO-331) shows recurrent leakage and intake chamber pressure drop. Recommended actions from the OEM Manual (doc_101) advise inspecting the bearing housing alignment and verifying intake filtration status to prevent shaft deflections that damage the seal face.",
      confidence: 0.88,
      citations: [
        {
          documentTitle: "Work Order WO-223 Seal Replacement.pdf",
          page: 1,
          snippet: "Primary mechanical seal failed, causing localized steam/water leakage. Shaft polished, seal replaced, aligned motor. Vibration measurements pending."
        },
        {
          documentTitle: "Incident Report IR-2026-04 Leakage.pdf",
          page: 2,
          snippet: "Feed pump P-101 showed fluid leakage on external seal flange. Operating temperature spikes observed 10 minutes prior to leakage."
        },
        {
          documentTitle: "Pump P-101 OEM Manual.pdf",
          page: 47,
          snippet: "Section 6.4: Repeated shaft seal leakage is typically caused by shaft deflection, misalignment, or bearing wear. Ensure radial misalignment is below 0.05 mm."
        }
      ],
      relatedAssets: ["P-101"],
      missingInfo: ["Post-installation vibration report (Missing Compliance Gap detected)", "Cooling water temperature logs during the June 2026 pressure drop"]
    };
  }

  if (q.includes('boiler') || q.includes('b-12') || q.includes('startup')) {
    return {
      answer: "Boiler B-12 startup procedure is outlined in SOP-BOI-04. The checklist requires verifying the feedwater valve status, executing pre-purge sequences for gas clearance, verifying flame scanner indicators, and keeping the ramp-up rate below 2.5°C/min. Operation of B-12 is currently in a high-risk state because its Hydrostatic Certificate expired on May 30, 2026 (Code B-12 rules).",
      confidence: 0.94,
      citations: [
        {
          documentTitle: "SOP-BOI-04 Boiler B-12 Startup.pdf",
          page: 2,
          snippet: "Pre-firing checklist: Verify feed drum level is 45-55%, start combustion air fans, purge furnace for 5 minutes to clear combustible gas pockets."
        },
        {
          documentTitle: "Safety Checklist Hot Work v2.pdf",
          page: 1,
          snippet: "Permit requirements: Gas test must show 0% LEL prior to any firing sequence inside the auxiliary boiler enclosure."
        }
      ],
      relatedAssets: ["B-12"],
      missingInfo: ["Hydrostatic Certificate active evidence (Expired May 30, 2026)"]
    };
  }

  if (q.includes('compressor') || q.includes('c-204') || q.includes('overheat')) {
    return {
      answer: "Compressor C-204 experienced an overheating incident on February 18, 2026 in its stage-2 discharge valve. The issue was resolved under WO-198 by cleaning carbon buildup, replacing gaskets, and verifying coolant flow rate. Standard specifications require a maximum discharge temperature of 135°C.",
      confidence: 0.91,
      citations: [
        {
          documentTitle: "Compressor C-204 Technical Specifications.pdf",
          page: 12,
          snippet: "Discharge valve temperatures above 135°C indicate cooling failures, valve seat carbon buildup, or gasket leakage. Clean immediately."
        }
      ],
      relatedAssets: ["C-204"],
      missingInfo: ["SOP training records for recent operators (Missing Compliance Gap detected)"]
    };
  }

  // Fallback
  return {
    answer: "I found matching reference documents containing details of plant assets, but no specific historical failure patterns that answer this question directly. The plant index shows 5 active assets (P-101, C-204, B-12, HX-204, V-301) and 10 related maintenance manuals, checklists, and audit reports. Please refine your search by adding asset tags or failure terms (e.g. seal leak, vibration, overheating).",
    confidence: 0.65,
    citations: [
      {
        documentTitle: "Plant Compliance Guidelines.pdf",
        page: 1,
        snippet: "All pressure vessels, high-speed rotating equipment, and critical safety valves must maintain updated inspection records and manuals."
      }
    ],
    relatedAssets: ["P-101", "C-204", "B-12", "HX-204", "V-301"],
    missingInfo: []
  };
};

export interface RcaReport {
  summary: string;
  fiveWhys: string[];
  probableCauses: string[];
  recommendations: string[];
  confidence: number;
  citations?: Array<{
    documentTitle: string;
    page?: number;
    snippet?: string;
  }>;
  missingData?: string[];
}

export const generateRca = (assetTag: string, failureDescription: string): RcaReport => {
  if (assetTag === 'P-101') {
    return {
      summary: "P-101 experienced recurrent mechanical seal failures culminating in pressure drop and fluid leaks. Structural evidence suggests vibration and shaft misalignment led to premature wear of the seal face.",
      fiveWhys: [
        "Why did the pump leak fluid? - Mechanical seal failed and seal face split.",
        "Why did the seal face split? - Heavy frictional heat and vibration deflection.",
        "Why was there vibration and frictional heat? - The pump shaft was experiencing dynamic misalignment.",
        "Why was the shaft misaligned? - Motor coupling was misaligned during the hasty seal replacement in March 2025.",
        "Why was it misaligned during installation? - Lack of vibration alignment checking post-install, and omission of the post-job vibration report."
      ],
      probableCauses: [
        "Radial shaft misalignment (>0.12 mm vs OEM recommended <0.05 mm limit)",
        "Inadequate post-maintenance verification and testing",
        "Abrasive particulates in feedwater wearing down the graphite seal ring"
      ],
      recommendations: [
        "Perform laser alignment of motor and pump shafts immediately.",
        "Enforce completion and upload of the post-maintenance vibration report before returning P-101 to service.",
        "Upgrade mechanical seal face material to silicon carbide (SiC/SiC) for high debris tolerance."
      ],
      confidence: 0.85,
      citations: [
        {
          documentTitle: "Work Order WO-223 Seal Replacement.pdf",
          page: 1,
          snippet: "Mechanical seal replaced, shaft polished, aligned motor. Vibration measurements pending."
        },
        {
          documentTitle: "Pump P-101 OEM Manual.pdf",
          page: 47,
          snippet: "Repeated shaft seal leakage is typically caused by shaft deflection, misalignment, or bearing wear."
        }
      ],
      missingData: [
        "Post-installation vibration report after WO-223",
        "Cooling water temperature logs during the June 2026 pressure drop"
      ]
    };
  }

  // Fallback
  return {
    summary: `RCA report generated for asset ${assetTag} concerning: ${failureDescription}. Preliminary evaluation suggests standard component wear compounded by operational stresses.`,
    fiveWhys: [
      `Why did the component fail? - High mechanical / thermal stress exceeded design limits.`,
      `Why did stress exceed design limits? - Operating conditions fluctuated or maintenance was delayed.`,
      `Why was maintenance delayed or condition unchecked? - Inspection intervals did not detect early warnings.`,
      `Why were early warnings missed? - Calibration or diagnostic reports were missing or not uploaded.`,
      `Why was the reports missing? - Asset tracking pipeline lacked automated compliance verification alerts.`
    ],
    probableCauses: [
      "Thermal or mechanical overload beyond standard OEM specifications",
      "Delayed preventive maintenance / lubrication intervals",
      "Lack of real-time diagnostic checks on the asset"
    ],
    recommendations: [
      "Upload and review standard operating procedures (SOP) for this asset class.",
      "Increase inspection frequency to bi-weekly until baseline operating conditions stabilize.",
      "Check calibration and active certifications of the asset."
    ],
    confidence: 0.72,
    citations: [],
    missingData: [
      "Asset-specific inspection report",
      "Recent work-order evidence"
    ]
  };
};
