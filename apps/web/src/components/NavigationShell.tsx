'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import {
  LayoutDashboard,
  FileText,
  Layers,
  MessageSquareCode,
  Activity,
  ShieldAlert,
  LogOut,
  User,
  Cpu,
  Network,
  FileDown,
  Settings,
  Menu,
  X,
  Lock,
  AlertTriangle,
} from 'lucide-react';
import { API_FORBIDDEN_EVENT, apiFetch, clearStoredSession, getStoredSession, PlantBrainSession } from '../lib/api';
import { canAccessPath, getDeniedMessage, getRoleLabel, getRouteAction } from '../lib/permissions';

interface NavigationShellProps {
  children: React.ReactNode;
}

const navItems = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Document Hub', href: '/documents', icon: FileText },
  { name: 'Asset Explorer', href: '/assets', icon: Layers },
  { name: 'RAG Copilot', href: '/copilot', icon: MessageSquareCode },
  { name: 'RCA Assistant', href: '/rca', icon: Activity },
  { name: 'Compliance Audit', href: '/compliance', icon: ShieldAlert },
  { name: 'Knowledge Graph', href: '/graph', icon: Network },
  { name: 'Reports', href: '/reports', icon: FileDown },
  { name: 'Admin Settings', href: '/admin', icon: Settings },
];

export default function NavigationShell({ children }: NavigationShellProps) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<PlantBrainSession | null>(null);
  const [loading, setLoading] = useState(true);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [accessNotice, setAccessNotice] = useState('');
  const [gatewayOnline, setGatewayOnline] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    const check = async () => {
      const res = await apiFetch('/api/me').catch(() => null);
      if (!cancelled) setGatewayOnline(Boolean(res?.ok));
    };
    check();
    const interval = setInterval(check, 30000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    const session = getStoredSession();
    if (!session) {
      router.push('/login');
      setLoading(false);
      return;
    }

    setUser(session);
    setLoading(false);
  }, [router]);

  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);

  useEffect(() => {
    const handleForbidden = (event: Event) => {
      const detail = (event as CustomEvent<{ method?: string; path?: string }>).detail;
      const target = detail?.path ? ` ${detail.path}` : '';
      setAccessNotice(`Your current role cannot access${target}.`);
    };

    window.addEventListener(API_FORBIDDEN_EVENT, handleForbidden);
    return () => window.removeEventListener(API_FORBIDDEN_EVENT, handleForbidden);
  }, []);

  const handleLogout = () => {
    clearStoredSession();
    router.push('/login');
  };

  const pageTitle = getPageTitle(pathname);

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-[#020617]">
        <div className="flex flex-col items-center gap-4">
          <Cpu className="w-12 h-12 text-cyber-emerald animate-spin" />
          <p className="text-slate-400 font-mono text-sm">Initializing Brain...</p>
        </div>
      </div>
    );
  }

  if (!user) return null;

  return (
    <div className="min-h-screen bg-[#020617] text-slate-100 font-sans lg:flex">
      {mobileOpen && (
        <button
          aria-label="Close navigation"
          className="fixed inset-0 z-30 bg-black/60 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <aside
        className={`fixed inset-y-0 left-0 z-40 w-72 max-w-[86vw] border-r border-slate-800/80 bg-[#090d1f] flex flex-col justify-between shrink-0 transition-transform duration-200 lg:static lg:w-64 lg:translate-x-0 ${
          mobileOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="min-h-0">
          <div className="h-16 flex items-center justify-between px-5 border-b border-slate-800/50 gap-2.5">
            <div className="flex items-center gap-2.5 min-w-0">
              <div className="bg-gradient-to-tr from-cyber-emerald to-cyber-blue p-2 rounded-lg shadow-[0_0_15px_rgba(16,185,129,0.3)] animate-pulse-glow">
                <Cpu className="w-5 h-5 text-white" />
              </div>
              <div className="min-w-0">
                <span className="font-extrabold text-white tracking-wider text-base">
                  PlantBrain<span className="text-cyber-emerald">AI</span>
                </span>
                <p className="text-[10px] text-slate-500 font-mono tracking-widest uppercase">OPS BRAIN v0.2</p>
              </div>
            </div>
            <button
              type="button"
              onClick={() => setMobileOpen(false)}
              className="lg:hidden p-2 rounded-lg border border-slate-800 text-slate-400 hover:text-white hover:border-slate-700"
              aria-label="Close menu"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <nav className="p-3 space-y-1.5 overflow-y-auto max-h-[calc(100vh-9rem)]">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = pathname === item.href || (item.href !== '/' && pathname.startsWith(item.href));
              const isAllowed = canAccessPath(user.role, item.href);
              const deniedReason = getDeniedMessage(user.role, getRouteAction(item.href));

              if (!isAllowed) {
                return (
                  <button
                    key={item.href}
                    type="button"
                    disabled
                    title={deniedReason}
                    className="w-full flex items-center gap-3 px-4 py-3 rounded-lg text-sm font-medium text-slate-600 border border-transparent cursor-not-allowed"
                  >
                    <Icon className="w-4 h-4 text-slate-700" />
                    <span className="truncate">{item.name}</span>
                    <Lock className="w-3.5 h-3.5 ml-auto text-slate-700" />
                  </button>
                );
              }

              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-all duration-200 text-sm font-medium ${
                    isActive
                      ? 'bg-gradient-to-r from-emerald-500/10 to-indigo-500/5 text-cyber-emerald border border-emerald-500/20 shadow-[0_0_20px_rgba(16,185,129,0.05)]'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/30 border border-transparent'
                  }`}
                >
                  <Icon className={`w-4 h-4 transition-colors ${isActive ? 'text-cyber-emerald' : 'text-slate-500'}`} />
                  <span className="truncate">{item.name}</span>
                </Link>
              );
            })}
          </nav>
        </div>

        <div className="p-4 border-t border-slate-800/60 bg-[#060918]">
          <div className="flex items-center gap-3 p-2 rounded-lg">
            <div className="bg-slate-800 border border-slate-700 p-1.5 rounded-full text-cyber-emerald">
              <User className="w-5 h-5" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-xs font-semibold text-slate-200 truncate">{user.name}</p>
              <p className="text-[10px] text-slate-500 truncate uppercase font-mono">{getRoleLabel(user.role)}</p>
              {user.plantName && <p className="text-[10px] text-slate-600 truncate font-mono">{user.plantName}</p>}
            </div>
            <button
              onClick={handleLogout}
              className="text-slate-500 hover:text-cyber-rose p-1.5 rounded hover:bg-slate-800/40 transition-colors"
              title="Sign Out"
              aria-label="Sign out"
            >
              <LogOut className="w-4 h-4" />
            </button>
          </div>
        </div>
      </aside>

      <main className="flex-1 flex flex-col min-w-0 min-h-screen">
        <header className="min-h-16 border-b border-slate-800/80 bg-[#090d1f]/80 backdrop-blur-md flex items-center justify-between gap-3 px-4 sm:px-6 lg:px-8 z-20 sticky top-0">
          <div className="flex items-center gap-3 min-w-0">
            <button
              type="button"
              onClick={() => setMobileOpen(true)}
              className="lg:hidden p-2 rounded-lg border border-slate-800 text-slate-300 hover:border-slate-700 hover:text-white"
              aria-label="Open menu"
            >
              <Menu className="w-5 h-5" />
            </button>
            <h1 className="text-sm sm:text-lg font-bold text-white tracking-wide truncate">{pageTitle}</h1>
          </div>

          <div className="hidden sm:flex items-center gap-4 lg:gap-6 text-[10px] lg:text-xs text-slate-400 font-mono">
            <div className="flex items-center gap-2">
              <span className={`w-2 h-2 rounded-full ${gatewayOnline === false ? 'bg-red-500' : gatewayOnline ? 'bg-cyber-emerald animate-ping' : 'bg-slate-500'}`} />
              <span>
                Gateway: <span className={`font-bold ${gatewayOnline === false ? 'text-red-400' : gatewayOnline ? 'text-cyber-emerald' : 'text-slate-400'}`}>
                  {gatewayOnline === null ? 'Checking…' : gatewayOnline ? 'Online' : 'Offline'}
                </span>
              </span>
            </div>
            <div className="hidden md:block h-4 w-px bg-slate-800" />
            <div className="hidden md:block">
              <span>
                Role: <span className="text-slate-300">{getRoleLabel(user.role)}</span>
              </span>
            </div>
          </div>
        </header>

        {accessNotice && (
          <div className="mx-4 sm:mx-6 lg:mx-8 mt-4 p-3 rounded-xl border border-cyber-amber/20 bg-cyber-amber/5 text-amber-200 text-xs flex items-start justify-between gap-3">
            <div className="flex items-start gap-2">
              <AlertTriangle className="w-4 h-4 text-cyber-amber shrink-0 mt-0.5" />
              <span>{accessNotice}</span>
            </div>
            <button
              type="button"
              onClick={() => setAccessNotice('')}
              className="text-amber-200/70 hover:text-amber-100"
              aria-label="Dismiss access notice"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        )}

        <div className="p-4 sm:p-6 lg:p-8 flex-1 max-w-[1600px] w-full mx-auto">{children}</div>
      </main>
    </div>
  );
}

function getPageTitle(pathname: string) {
  if (pathname === '/') return 'Operational Dashboard';
  if (pathname.startsWith('/documents')) return 'Document Ingestion Hub';
  if (pathname.startsWith('/assets')) return 'Asset Intelligence Explorer';
  if (pathname === '/copilot') return 'Industrial RAG Copilot';
  if (pathname === '/rca') return 'Root Cause Analysis (RCA) Assistant';
  if (pathname === '/compliance') return 'Compliance Audit Center';
  if (pathname === '/graph') return 'Knowledge Graph';
  if (pathname === '/reports') return 'Reports';
  if (pathname === '/admin') return 'Admin Settings';
  return 'System Console';
}
