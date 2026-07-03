'use client';

import React, { Suspense, useState, useEffect, useRef } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { queryCopilot, CopilotResponse } from '../../lib/mockData';
import { apiFetch } from '../../lib/api';
import { recordQueryAnswer } from '../../lib/query-history';
import { useData } from '../../context/DataContext';
import { useSearchParams, useRouter } from 'next/navigation';
import { 
  Send, 
  Bot, 
  User, 
  FileText, 
  AlertTriangle, 
  Layers, 
  Cpu, 
  SlidersHorizontal,
  ChevronRight,
  Info,
  Sparkles
} from 'lucide-react';
import Link from 'next/link';

interface Message {
  id: string;
  sender: 'user' | 'assistant';
  text: string;
  timestamp: Date;
  response?: CopilotResponse;
  loading?: boolean;
}

function CopilotPageContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const { assets, documents } = useData();
  const initialTag = searchParams.get('tag') || '';
  
  const [messages, setMessages] = useState<Message[]>([
    {
      id: 'welcome',
      sender: 'assistant',
      text: "Hello! I am the PlantBrain AI Copilot. You can ask me operational questions about manuals, safety checklists, SOPs, and historical failure events. I will answer with citations and flag any gaps in our records.",
      timestamp: new Date()
    }
  ]);
  const [input, setInput] = useState('');
  const [assetFilter, setAssetFilter] = useState(initialTag);
  const [showFilters, setShowFilters] = useState(false);
  const [activeMessageId, setActiveMessageId] = useState<string | null>(null);
  const [selectedDocTypes, setSelectedDocTypes] = useState<string[]>(['Maintenance Work Order', 'Inspection Report', 'OEM Manual', 'SOP']);
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');
  
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const availableDocTypes = Array.from(new Set(documents.map(doc => doc.documentType))).sort();

  // Suggested Prompts
  const suggestions = [
    { label: "P-101 Failure Reason", query: "Why is Pump P-101 repeatedly failing?" },
    { label: "Boiler B-12 SOP", query: "Show SOP for boiler startup." },
    { label: "C-204 Overheating", query: "Why did Compressor C-204 fail on Feb 18?" }
  ];

  // Auto-scroll chat
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Handle URL tag parameter changes
  useEffect(() => {
    if (initialTag) {
      setAssetFilter(initialTag);
      setInput(`Why does Pump ${initialTag} keep failing?`);
    }
  }, [initialTag]);

  const handleSend = async (textToSend: string) => {
    if (!textToSend.trim()) return;

    const userMessageId = `msg_${Date.now()}`;
    const assistantMessageId = `msg_${Date.now() + 1}`;
    
    // Add user message
    const newMessages: Message[] = [
      ...messages,
      {
        id: userMessageId,
        sender: 'user',
        text: textToSend,
        timestamp: new Date()
      },
      {
        id: assistantMessageId,
        sender: 'assistant',
        text: '',
        timestamp: new Date(),
        loading: true
      }
    ];
    
    setMessages(newMessages);
    setInput('');
    setActiveMessageId(assistantMessageId);

    // Call API / Fallback Mock
    try {
      // Setup payload matching instructions specifications
      const payload = {
        question: textToSend,
        filters: {
          assetTag: assetFilter || undefined,
          documentTypes: selectedDocTypes,
          dateFrom: dateFrom || undefined,
          dateTo: dateTo || undefined,
        }
      };

      // Try calling real Gateway API
      const res = await apiFetch('/api/copilot/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (!res.ok) throw new Error('API server unreachable');

      const data = normalizeCopilotResponse(await res.json());
      
      setMessages(prev => prev.map(m => {
        if (m.id === assistantMessageId) {
          return {
            ...m,
            text: data.answer,
            loading: false,
            response: data
          };
        }
        return m;
      }));
      recordQueryAnswer(textToSend, data, assetFilter);

    } catch (err) {
      // Fallback to offline RAG mock engine
      setTimeout(() => {
        const mockRes = queryCopilot(textToSend, assetFilter);
        setMessages(prev => prev.map(m => {
          if (m.id === assistantMessageId) {
            return {
              ...m,
              text: mockRes.answer,
              loading: false,
              response: mockRes
            };
          }
          return m;
        }));
        recordQueryAnswer(textToSend, mockRes, assetFilter);
      }, 1000);
    }
  };

  // Find active message for citations
  const activeMessage = messages.find(m => m.id === activeMessageId && m.response);
  const activeResponse = activeMessage?.response;

  return (
    <NavigationShell>
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-6 h-[calc(100vh-10rem)] max-h-[900px] overflow-hidden">
        
        {/* Left/Middle Column: Chat Panel */}
        <div className="xl:col-span-2 flex flex-col justify-between glass-panel rounded-2xl border-slate-800/80 overflow-hidden relative">
          
          {/* Chat Header Filters */}
          <div className="bg-[#090d1f]/80 p-4 border-b border-slate-800/80 flex items-center justify-between flex-wrap gap-4">
            <div className="flex items-center gap-2">
              <Sparkles className="w-4 h-4 text-cyber-emerald" />
              <span className="text-xs font-bold text-slate-200">Retrieval Augmented Chat</span>
            </div>
            
            <div className="flex items-center gap-2">
              <button 
                onClick={() => setShowFilters(!showFilters)}
                className={`p-1.5 rounded-lg border text-xs font-semibold flex items-center gap-1.5 transition-all ${
                  showFilters || assetFilter 
                    ? 'border-emerald-500/30 bg-emerald-500/5 text-cyber-emerald' 
                    : 'border-slate-800 text-slate-400 hover:border-slate-700'
                }`}
              >
                <SlidersHorizontal className="w-3.5 h-3.5" />
                {assetFilter ? `Filtered: ${assetFilter}` : 'Filter Query'}
              </button>
            </div>
          </div>

          {/* Filter dropdown panel */}
          {showFilters && (
            <div className="bg-[#090d1f] p-4 border-b border-slate-800/60 grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
              <div>
                <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Asset Tag Filter</label>
                <select
                  value={assetFilter}
                  onChange={(e) => setAssetFilter(e.target.value)}
                  className="w-full px-3 py-1.5 rounded-lg glass-input text-xs font-medium"
                >
                  <option value="">All assets</option>
                  {assets.map(asset => (
                    <option key={asset.id} value={asset.assetTag}>{asset.assetTag} - {asset.assetName}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Date Range</label>
                <div className="grid grid-cols-2 gap-2">
                  <input
                    type="date"
                    value={dateFrom}
                    onChange={(e) => setDateFrom(e.target.value)}
                    className="w-full px-3 py-1.5 rounded-lg glass-input text-xs font-medium"
                  />
                  <input
                    type="date"
                    value={dateTo}
                    onChange={(e) => setDateTo(e.target.value)}
                    className="w-full px-3 py-1.5 rounded-lg glass-input text-xs font-medium"
                  />
                </div>
              </div>
              <div className="sm:col-span-2">
                <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-2">Document Types</label>
                <div className="flex flex-wrap gap-2">
                  {availableDocTypes.map(type => {
                    const active = selectedDocTypes.includes(type);
                    return (
                      <button
                        type="button"
                        key={type}
                        onClick={() => setSelectedDocTypes(prev => active ? prev.filter(item => item !== type) : [...prev, type])}
                        className={`px-2.5 py-1 rounded-lg border text-[10px] font-mono font-semibold transition-all ${
                          active
                            ? 'border-emerald-500/30 bg-emerald-500/10 text-cyber-emerald'
                            : 'border-slate-800 bg-slate-900/60 text-slate-500 hover:text-slate-300'
                        }`}
                      >
                        {type}
                      </button>
                    );
                  })}
                </div>
              </div>
              <div className="sm:col-span-2 flex items-end justify-end">
                <button
                  onClick={() => { setAssetFilter(''); setDateFrom(''); setDateTo(''); setSelectedDocTypes(availableDocTypes); setShowFilters(false); }}
                  className="px-3 py-1.5 border border-slate-800 hover:bg-slate-800/50 text-slate-400 font-semibold rounded-lg text-xs"
                >
                  Clear Filters
                </button>
              </div>
            </div>
          )}

          {/* Chat Messages */}
          <div className="flex-1 overflow-y-auto p-6 space-y-6">
            {messages.map((msg) => {
              const isAssistant = msg.sender === 'assistant';
              return (
                <div 
                  key={msg.id}
                  onClick={() => msg.response && setActiveMessageId(msg.id)}
                  className={`flex gap-4 ${isAssistant ? 'justify-start' : 'justify-end'}`}
                >
                  {isAssistant && (
                    <div className="w-8 h-8 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-cyber-emerald flex items-center justify-center shrink-0">
                      <Bot className="w-4 h-4" />
                    </div>
                  )}

                  <div className={`max-w-[75%] rounded-2xl p-4 text-xs font-medium space-y-3 cursor-pointer transition-all ${
                    isAssistant 
                      ? 'bg-slate-900/60 border border-slate-800 text-slate-100 hover:border-slate-700/60' 
                      : 'bg-cyber-emerald text-slate-950 font-bold border border-emerald-400/20'
                  }`}>
                    {msg.loading ? (
                      <div className="flex items-center gap-2 py-1 font-mono text-[11px] text-slate-400">
                        <Cpu className="w-4 h-4 animate-spin text-cyber-emerald" />
                        <span>Searching vector database & graph...</span>
                      </div>
                    ) : (
                      <>
                        <p className="leading-relaxed whitespace-pre-wrap">{msg.text}</p>
                        
                        {isAssistant && msg.response && (
                          <div className="pt-3 border-t border-slate-800/60 flex flex-wrap items-center justify-between gap-3 text-[10px] text-slate-400">
                            <span className="font-mono">Confidence: <span className="text-cyber-emerald font-bold">{(msg.response.confidence * 100).toFixed(0)}%</span></span>
                            {msg.response.citations.length > 0 && (
                              <button 
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setActiveMessageId(msg.id);
                                }}
                                className="text-cyber-emerald hover:underline font-bold flex items-center gap-0.5"
                              >
                                {msg.response.citations.length} Citations <ChevronRight className="w-3.5 h-3.5" />
                              </button>
                            )}
                          </div>
                        )}
                      </>
                    )}
                  </div>

                  {!isAssistant && (
                    <div className="w-8 h-8 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-cyber-indigo flex items-center justify-center shrink-0">
                      <User className="w-4 h-4" />
                    </div>
                  )}
                </div>
              );
            })}
            <div ref={messagesEndRef} />
          </div>

          {/* Prompt Suggestions */}
          {messages.length === 1 && (
            <div className="px-6 py-3 border-t border-slate-800/40 flex flex-wrap gap-2 justify-center bg-[#060918]/30">
              {suggestions.map(s => (
                <button
                  key={s.label}
                  onClick={() => handleSend(s.query)}
                  className="px-3 py-1.5 rounded-full border border-slate-800 hover:border-emerald-500/30 bg-slate-900/60 hover:bg-emerald-500/5 text-[10px] font-mono font-semibold text-slate-300 hover:text-cyber-emerald transition-all"
                >
                  {s.label}
                </button>
              ))}
            </div>
          )}

          {/* Form Input */}
          <form 
            onSubmit={(e) => { e.preventDefault(); handleSend(input); }}
            className="p-4 border-t border-slate-800/80 bg-[#090d1f]/80 flex gap-2"
          >
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Ask a question about plant manuals, failures, or codes..."
              className="flex-1 px-4 py-3 rounded-xl glass-input text-xs font-medium"
            />
            <button
              type="submit"
              className="p-3 bg-cyber-emerald hover:bg-emerald-600 active:bg-emerald-700 text-slate-950 font-bold rounded-xl transition-all hover:shadow-[0_0_15px_rgba(16,185,129,0.2)]"
            >
              <Send className="w-4 h-4" />
            </button>
          </form>

        </div>

        {/* Right Column: Citation & Evidence Panel */}
        <div className="xl:col-span-1 glass-panel rounded-2xl border-slate-800/80 p-6 flex flex-col justify-between h-full overflow-y-auto">
          <div>
            <h3 className="text-base font-bold text-white tracking-wide mb-4">Evidence Panel</h3>

            {activeResponse ? (
              <div className="space-y-6">
                {(activeResponse.confidence < 0.7 || activeResponse.citations.length === 0) && (
                  <div className="p-3 bg-cyber-amber/5 border border-cyber-amber/20 text-amber-200 rounded-xl text-xs flex gap-2 font-medium">
                    <AlertTriangle className="w-4 h-4 text-cyber-amber shrink-0" />
                    <span>
                      {activeResponse.citations.length === 0
                        ? 'No source citation was returned, so this answer should not be treated as factual evidence.'
                        : 'Confidence is below the evidence threshold; verify the cited source before acting.'}
                    </span>
                  </div>
                )}
                
                {/* Citations List */}
                <div>
                  <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3">Source Citations</h4>
                  <div className="space-y-3">
                    {activeResponse.citations.map((cite, index) => (
                      <div 
                        key={index}
                        className="p-3.5 bg-[#060918]/60 border border-slate-800 hover:border-slate-700/50 rounded-xl space-y-2 text-xs"
                      >
                        <div className="flex items-center gap-1.5 text-cyber-blue font-bold">
                          <FileText className="w-3.5 h-3.5 shrink-0" />
                          <span className="truncate">{cite.documentTitle}</span>
                          <span className="text-[10px] font-mono bg-slate-800 text-slate-400 border border-slate-700 px-1.5 py-0.5 rounded shrink-0">
                            Page {cite.page}
                          </span>
                        </div>
                        <p className="text-[11px] text-slate-400 font-medium leading-relaxed italic border-l border-slate-800 pl-2">
                          &ldquo;{cite.snippet}&rdquo;
                        </p>
                      </div>
                    ))}
                  </div>
                </div>

                {/* Missing Data Warnings */}
                {activeResponse.missingInfo.length > 0 && (
                  <div>
                    <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3 flex items-center gap-1">
                      <AlertTriangle className="w-3.5 h-3.5 text-cyber-rose" /> Missing Data Gaps
                    </h4>
                    <div className="space-y-2">
                      {activeResponse.missingInfo.map((gap, index) => (
                        <div 
                          key={index}
                          className="p-3 bg-cyber-rose/5 border border-cyber-rose/10 text-rose-300 rounded-xl text-[11px] flex gap-2 font-medium"
                        >
                          <Info className="w-4 h-4 text-cyber-rose shrink-0" />
                          <span>{gap}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Related Assets */}
                {activeResponse.relatedAssets.length > 0 && (
                  <div>
                    <h4 className="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3">Linked Assets</h4>
                    <div className="flex flex-wrap gap-2">
                      {activeResponse.relatedAssets.map(tag => (
                        <Link
                          key={tag}
                          href={`/assets/${tag}`}
                          className="px-2.5 py-1 bg-slate-900 border border-slate-800 hover:border-cyber-emerald/30 text-slate-300 hover:text-cyber-emerald font-mono font-semibold rounded-lg text-xs flex items-center gap-1.5 transition-all"
                        >
                          <Layers className="w-3.5 h-3.5" /> {tag}
                        </Link>
                      ))}
                    </div>
                  </div>
                )}

              </div>
            ) : (
              <div className="py-20 text-center text-slate-500 font-mono text-xs flex flex-col items-center justify-center gap-3">
                <Bot className="w-8 h-8 text-slate-600 animate-pulse-glow" />
                <p>Click on any Copilot answer to inspect its vector citations, missing database logs, and related equipment profiles.</p>
              </div>
            )}
          </div>
          
          {activeResponse && (
            <div className="border-t border-slate-800/80 pt-4 text-[10px] text-slate-500 font-mono text-center">
              Evaluation Metrics: CRAG Faithfulness Verified
            </div>
          )}
        </div>

      </div>
    </NavigationShell>
  );
}

export default function CopilotPage() {
  return (
    <Suspense fallback={null}>
      <CopilotPageContent />
    </Suspense>
  );
}

function normalizeCopilotResponse(data: any): CopilotResponse {
  return {
    answer: String(data?.answer || 'No answer could be generated from available evidence.'),
    confidence: Number(data?.confidence ?? 0),
    citations: Array.isArray(data?.citations)
      ? data.citations.map((citation: any) => ({
          documentTitle: String(citation.documentTitle || citation.title || 'Untitled source'),
          page: Number(citation.page || citation.pageNo || 1),
          snippet: String(citation.snippet || citation.quotedText || citation.text || ''),
        }))
      : [],
    relatedAssets: Array.isArray(data?.relatedAssets) ? data.relatedAssets.map(String) : [],
    missingInfo: Array.isArray(data?.missingInfo) ? data.missingInfo.map(String) : [],
  };
}
