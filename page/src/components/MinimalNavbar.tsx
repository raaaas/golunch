import { useState } from 'react';
import { Terminal, Github, Heart, Copy, Check } from 'lucide-react';

interface MinimalNavbarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
  onHomeClick?: () => void;
}

export const MinimalNavbar = ({ activeTab, setActiveTab, onHomeClick }: MinimalNavbarProps) => {
  const [copied, setCopied] = useState(false);

  const copyInstall = () => {
    navigator.clipboard.writeText('go install github.com/raaaas/golunch/cmd/golunch@latest');
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const navs = [
    { id: 'playground', label: 'playground' },
    { id: 'tasks', label: 'tasks' },
    { id: 'isolation', label: 'isolation' },
    { id: 'docs', label: 'docs' },
  ];

  return (
    <header className="sticky top-0 z-50 w-full bg-[#141414]/90 backdrop-blur-md border-b border-[#222222]">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 h-14 flex items-center justify-between font-mono text-[13px]">
        
        {/* Brand (scroll to top) */}
        <button
          onClick={onHomeClick || (() => setActiveTab('playground'))}
          className="flex items-center gap-2 group text-left cursor-pointer"
          title="Go to top"
        >
          <span className="w-2.5 h-2.5 rounded-full bg-[#e17a56] shadow-[0_0_8px_#e17a56] group-hover:scale-125 transition-transform duration-200"></span>
          <span className="font-bold tracking-tight text-[#ece8e2] font-mono text-sm">
            golunch
          </span>
          <span className="text-[#8d8983] text-[11px] hidden sm:inline">
            // multi-agent harness
          </span>
        </button>

        {/* Navigation Tabs (active click handler with visual feedback) */}
        <nav className="flex items-center gap-1 sm:gap-2">
          {navs.map((n) => (
            <button
              key={n.id}
              onClick={() => setActiveTab(n.id)}
              className={`px-2.5 py-1 rounded transition-all cursor-pointer text-xs ${
                activeTab === n.id
                  ? 'text-[#e17a56] font-medium bg-[#e17a56]/15 border border-[#e17a56]/40 shadow-sm'
                  : 'text-[#8d8983] hover:text-[#ece8e2] hover:bg-[#1a1a1a]'
              }`}
            >
              {n.label}
            </button>
          ))}
        </nav>

        {/* Quick actions */}
        <div className="flex items-center gap-2">
          <button
            onClick={copyInstall}
            className="hidden sm:inline-flex items-center gap-1.5 px-2 py-1 text-xs text-[#8d8983] hover:text-[#ece8e2] hover:bg-[#1c1c1c] rounded transition-colors cursor-pointer"
            title="Copy go install command"
          >
            {copied ? (
              <>
                <Check className="w-3.5 h-3.5 text-[#e17a56]" />
                <span className="text-[#e17a56]">copied</span>
              </>
            ) : (
              <>
                <Terminal className="w-3.5 h-3.5 text-[#8d8983]" />
                <span>install</span>
              </>
            )}
          </button>

          <a
            href="https://github.com/raaaas/golunch"
            target="_blank"
            rel="noopener noreferrer"
            className="p-1.5 text-[#8d8983] hover:text-[#ece8e2] rounded transition-colors"
            title="GitHub Repository"
          >
            <Github className="w-4 h-4" />
          </a>
        </div>

      </div>
    </header>
  );
};
