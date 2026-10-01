import { useState } from 'react';
import { Copy, Check, ArrowRight, ShieldCheck, Cpu, Terminal as TerminalIcon } from 'lucide-react';
import { TerminalSimulation } from './TerminalSimulation.tsx';

interface HeroProps {
  onNavigate: (tabId: string) => void;
}

export const Hero = ({ onNavigate }: HeroProps) => {
  const [copied, setCopied] = useState(false);
  const installCmd = 'go install github.com/raaaas/golunch/cmd/golunch@latest';

  const copyToClipboard = () => {
    navigator.clipboard.writeText(installCmd);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <section className="relative pt-12 pb-16 lg:pt-20 lg:pb-24 overflow-hidden">
      {/* Background radial glow */}
      <div className="absolute top-0 left-1/2 -translate-x-1/2 w-full max-w-7xl h-96 bg-gradient-to-b from-amber-500/10 via-transparent to-transparent pointer-events-none blur-3xl -z-10" />

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        
        {/* Unboxed editorial kicker / metadata */}
        <div className="flex items-center gap-2.5 text-xs text-neutral-400 font-mono mb-4 tracking-wide">
          <span className="text-amber-400 font-semibold">GOLUNCH HARNESS</span>
          <span aria-hidden="true" className="text-neutral-600">·</span>
          <span>GO 1.27+</span>
          <span aria-hidden="true" className="text-neutral-600">·</span>
          <span>LINUX & MACOS</span>
          <span aria-hidden="true" className="text-neutral-600">·</span>
          <span>MIT LICENSED</span>
        </div>

        {/* Headline with text-wrap: balance and typography discipline */}
        <div className="max-w-4xl">
          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-extrabold tracking-tight text-white font-display leading-[1.1] [text-wrap:balance]">
            Run the same agent CLI as many times as you want.
          </h1>

          <p className="mt-5 text-lg sm:text-xl text-neutral-300 font-normal leading-relaxed max-w-3xl [text-wrap:balance]">
            Each instance with its own private config, login, and proxy. No containers, no Linux namespaces, no background daemon. Pure process execution with kernel-managed advisory flock.
          </p>
        </div>

        {/* Interactive Primary Actions & Install Command */}
        <div className="mt-8 flex flex-col sm:flex-row items-stretch sm:items-center gap-4">
          {/* Quick install box */}
          <div className="flex items-center justify-between gap-3 px-4 py-2.5 rounded-lg bg-neutral-900 border border-neutral-800 text-sm font-mono text-neutral-200 shadow-inner group hover:border-neutral-700 transition-colors">
            <div className="flex items-center gap-2 overflow-hidden">
              <span className="text-emerald-400 select-none">$</span>
              <span className="truncate select-all">{installCmd}</span>
            </div>
            <button
              onClick={copyToClipboard}
              className="p-1.5 rounded hover:bg-neutral-800 text-neutral-400 hover:text-white transition-colors cursor-pointer shrink-0"
              title="Copy to clipboard"
            >
              {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
            </button>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={() => onNavigate('sandbox')}
              className="flex-1 sm:flex-initial inline-flex items-center justify-center gap-2 px-5 py-2.5 rounded-lg bg-amber-500 hover:bg-amber-400 text-neutral-950 font-semibold text-sm transition-all duration-150 shadow-sm cursor-pointer whitespace-nowrap active:scale-95"
            >
              <TerminalIcon className="w-4 h-4" />
              <span>Interactive CLI</span>
              <ArrowRight className="w-4 h-4" />
            </button>

            <button
              onClick={() => onNavigate('docs')}
              className="flex-1 sm:flex-initial inline-flex items-center justify-center gap-2 px-5 py-2.5 rounded-lg bg-neutral-900 hover:bg-neutral-800 text-neutral-200 border border-neutral-800 font-medium text-sm transition-all duration-150 cursor-pointer whitespace-nowrap"
            >
              <span>Documentation</span>
            </button>
          </div>
        </div>

        {/* Claim-to-Proof Quick Metrics (unboxed, clean typography) */}
        <div className="mt-10 pt-6 border-t border-neutral-800/80 grid grid-cols-2 md:grid-cols-4 gap-6 text-sm">
          <div>
            <div className="text-2xl font-bold font-mono text-white tabular-nums">0 ms</div>
            <div className="text-neutral-400 text-xs mt-0.5">Daemon or container startup delay</div>
          </div>
          <div>
            <div className="text-2xl font-bold font-mono text-white tabular-nums">8 Vars</div>
            <div className="text-neutral-400 text-xs mt-0.5">Dual-case HTTP/HTTPS proxy injection</div>
          </div>
          <div>
            <div className="text-2xl font-bold font-mono text-white tabular-nums">POSIX flock</div>
            <div className="text-neutral-400 text-xs mt-0.5">Kernel-released locks (zero stale files)</div>
          </div>
          <div>
            <div className="text-2xl font-bold font-mono text-white tabular-nums">100% Native</div>
            <div className="text-neutral-400 text-xs mt-0.5">No root or namespaces required</div>
          </div>
        </div>

        {/* Dominant Visual Carrier: Interactive Terminal */}
        <div className="mt-12">
          <TerminalSimulation />
        </div>

      </div>
    </section>
  );
};
