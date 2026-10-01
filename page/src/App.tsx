import { useState } from 'react';
import { MinimalNavbar } from './components/MinimalNavbar.tsx';
import { MinimalHero } from './components/MinimalHero.tsx';
import { MinimalTerminal } from './components/MinimalTerminal.tsx';
import { MinimalPlayground } from './components/MinimalPlayground.tsx';
import { MinimalTaskSweep } from './components/MinimalTaskSweep.tsx';
import { MinimalIsolation } from './components/MinimalIsolation.tsx';
import { MinimalDocs } from './components/MinimalDocs.tsx';
import { MinimalFooter } from './components/MinimalFooter.tsx';

export default function App() {
  const [activeTab, setActiveTab] = useState<string>('playground');

  return (
    <div className="min-h-screen bg-[#141414] text-[#ece8e2] flex flex-col font-mono selection:bg-[#e17a56]/30 selection:text-[#ffb08a]">
      {/* Sleek minimal header */}
      <MinimalNavbar activeTab={activeTab} setActiveTab={setActiveTab} />

      {/* Main container — centered, breathing room */}
      <main className="flex-1 max-w-4xl w-full mx-auto px-4 sm:px-6">
        
        {/* Minimal Hero with floating process scene */}
        <MinimalHero onSelectInstance={() => setActiveTab('isolation')} />

        {/* Minimal Terminal player */}
        <MinimalTerminal />

        {/* View Switcher Bar */}
        <div className="pt-6 pb-2 border-b border-[#222222] flex items-center justify-between text-xs text-[#8d8983]">
          <div className="flex items-center gap-1 sm:gap-2">
            {[
              { id: 'playground', label: '01. CLI Playground' },
              { id: 'tasks', label: '02. Task Matrix' },
              { id: 'isolation', label: '03. Isolation Engine' },
              { id: 'docs', label: '04. Documentation' }
            ].map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`px-3 py-1.5 rounded transition-all cursor-pointer ${
                  activeTab === tab.id
                    ? 'bg-[#1e1e1e] text-[#e17a56] font-bold border border-[#e17a56]/40'
                    : 'hover:text-[#ece8e2]'
                }`}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <span className="hidden sm:inline text-[11px] text-[#555]">
            tab: {activeTab}
          </span>
        </div>

        {/* Dynamic section display */}
        {activeTab === 'playground' && <MinimalPlayground />}
        {activeTab === 'tasks' && <MinimalTaskSweep />}
        {activeTab === 'isolation' && <MinimalIsolation />}
        {activeTab === 'docs' && <MinimalDocs />}

      </main>

      {/* Sleek minimal footer */}
      <MinimalFooter />
    </div>
  );
}
