import { useState } from 'react';
import { Terminal, Github, Heart, Copy, Check, Menu, X } from 'lucide-react';

interface NavbarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
}

export const Navbar = ({ activeTab, setActiveTab }: NavbarProps) => {
  const [copiedInstall, setCopiedInstall] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const copyInstallCmd = () => {
    navigator.clipboard.writeText('go install github.com/raaaas/golunch/cmd/golunch@latest');
    setCopiedInstall(true);
    setTimeout(() => setCopiedInstall(false), 2000);
  };

  const navItems = [
    { id: 'overview', label: 'Overview' },
    { id: 'architecture', label: 'Architecture' },
    { id: 'sandbox', label: 'CLI Sandbox' },
    { id: 'tasks', label: 'Task Matrix' },
    { id: 'docs', label: 'Documentation' },
  ];

  return (
    <header className="sticky top-0 z-50 w-full bg-neutral-950/85 backdrop-blur-md border-b border-neutral-800/80">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
        
        {/* Zone 1: Single text element Brand Zone */}
        <div className="flex items-center gap-3">
          <a
            href="#overview"
            onClick={(e) => {
              e.preventDefault();
              setActiveTab('overview');
            }}
            className="flex items-center gap-2.5 group cursor-pointer"
          >
            <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-amber-500 to-amber-600 flex items-center justify-center text-neutral-950 font-mono font-bold text-sm shadow-sm group-hover:scale-105 transition-transform duration-150">
              gl
            </div>
            <span className="text-xl font-bold tracking-tight text-white font-mono flex items-center gap-1.5">
              golunch
              <span className="inline-block w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            </span>
          </a>
        </div>

        {/* Zone 2: 4-6 clean text navigation links */}
        <nav className="hidden md:flex items-center gap-7 text-sm font-medium text-neutral-400">
          {navItems.map((item) => (
            <button
              key={item.id}
              onClick={() => setActiveTab(item.id)}
              className={`transition-colors whitespace-nowrap cursor-pointer py-1 border-b-2 ${
                activeTab === item.id
                  ? 'text-white border-amber-500 font-semibold'
                  : 'text-neutral-400 border-transparent hover:text-neutral-200'
              }`}
            >
              {item.label}
            </button>
          ))}
        </nav>

        {/* Zone 3: 1-2 primary actions */}
        <div className="flex items-center gap-3">
          <button
            onClick={copyInstallCmd}
            className="hidden sm:inline-flex items-center gap-2 px-3 py-1.5 text-xs font-mono text-neutral-300 bg-neutral-900 border border-neutral-700/80 rounded-md hover:bg-neutral-800 hover:text-white transition-all duration-150 active:scale-95 cursor-pointer whitespace-nowrap"
            title="Copy installation command"
          >
            <Terminal className="w-3.5 h-3.5 text-amber-400" />
            <span className="hidden lg:inline text-neutral-400">go install</span>
            <span>golunch@latest</span>
            {copiedInstall ? (
              <Check className="w-3.5 h-3.5 text-emerald-400 ml-1" />
            ) : (
              <Copy className="w-3.5 h-3.5 text-neutral-400 ml-1" />
            )}
          </button>

          <a
            href="https://github.com/raaaas/golunch"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-neutral-950 bg-white rounded-md hover:bg-neutral-200 transition-colors shadow-sm whitespace-nowrap"
          >
            <Github className="w-3.5 h-3.5" />
            <span>GitHub</span>
          </a>

          <a
            href="https://github.com/sponsors/raaaas"
            target="_blank"
            rel="noopener noreferrer"
            className="hidden xl:inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-pink-300 bg-pink-950/40 border border-pink-800/50 rounded-md hover:bg-pink-900/40 transition-colors whitespace-nowrap"
          >
            <Heart className="w-3.5 h-3.5 text-pink-400 fill-pink-400/20" />
            <span>Sponsor</span>
          </a>

          {/* Mobile hamburger */}
          <button
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            className="md:hidden p-2 text-neutral-400 hover:text-white focus:outline-none"
            aria-label="Toggle navigation"
          >
            {mobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
          </button>
        </div>
      </div>

      {/* Mobile menu dropdown */}
      {mobileMenuOpen && (
        <div className="md:hidden bg-neutral-900 border-b border-neutral-800 px-4 pt-3 pb-4 space-y-1">
          {navItems.map((item) => (
            <button
              key={item.id}
              onClick={() => {
                setActiveTab(item.id);
                setMobileMenuOpen(false);
              }}
              className={`block w-full text-left px-3 py-2 rounded-md text-sm font-medium ${
                activeTab === item.id
                  ? 'bg-neutral-800 text-amber-400'
                  : 'text-neutral-300 hover:bg-neutral-800/60'
              }`}
            >
              {item.label}
            </button>
          ))}
          <div className="pt-2 border-t border-neutral-800 flex flex-col gap-2">
            <button
              onClick={copyInstallCmd}
              className="flex items-center justify-between px-3 py-2 text-xs font-mono bg-neutral-950 rounded text-neutral-300 border border-neutral-800"
            >
              <span>go install .../cmd/golunch@latest</span>
              {copiedInstall ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4 text-neutral-400" />}
            </button>
          </div>
        </div>
      )}
    </header>
  );
};
