import '../styles/globals.css';
import { DataProvider } from '../context/DataContext';

export const metadata = {
  title: 'PlantBrainAI — Unified Asset & Operations Brain',
  description: 'AI-powered industrial knowledge intelligence platform',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&family=JetBrains+Mono:wght@400;500;700&display=swap" rel="stylesheet" />
      </head>
      <body className="antialiased min-h-screen text-slate-100 bg-[#020617]">
        <DataProvider>
          {children}
        </DataProvider>
      </body>
    </html>
  );
}
