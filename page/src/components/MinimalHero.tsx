import { useState } from 'react';
import { Terminal, Copy, Check, ArrowRight, Shield, Zap, Lock } from 'lucide-react';

interface InstanceNode {
  alias: string;
  agent: string;
  proxy: string;
  status: 'idle' | 'running' | 'locked';
  flock: 'LOCK_SH' | 'LOCK_EX' | 'none';
  x: string;
  y: string;
  dur: string;
}

const NODES: InstanceNode[] = [
  {
    alias: 'work',
    agent: 'cline',
    proxy: '127.0.0.1:7890',
    status: 'running',
    flock: 'LOCK_SH',
    x: 'left-4 sm:left-12',
    y: 'top-6',
    dur: '6s'
  },
  {
    alias: 'evals',
    agent: 'kilo',
    proxy: 'none (direct)',
    status: 'idle',
    flock: 'none',
    x: 'right-4 sm:right-16',
    y: 'top-10',
    dur: '7.5s'
  },
  {
    alias: 'audit',
    agent: 'opencode',
    proxy: 'corp:8888',
    status: 'locked',
    flock: 'LOCK_EX',
    x: 'left-1/3',
    y: 'top-28',
    dur: '8s'
  }
];

interface MinimalHeroProps {
  onSelectInstance?: (alias: string) => void;
}

export const MinimalHero = ({ onSelectInstance }: MinimalHeroProps) => {
  const [copied, setCopied] = useState(false);
  const [focusedNode, setFocusedNode] = useState<InstanceNode>(NODES[0]);

  const copyInstall = () => {
    navigator.clipboard.writeText('go install github.com/raaaas/golunch/cmd/golunch@latest');
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="pt-10 pb-8 sm:pt-14 sm:pb-12 text-left">
      
      {/* Top minimal status bar */}
      <div className="flex items-center justify-between text-xs text-[#8d8983] mb-4 pb-2 border-b border-[#222222]">
        <div className="flex items-center gap-2">
          <span className="text-[#e17a56] font-bold">GOLUNCH</span>
          <span>·</span>
          <span>v1.0.0</span>
          <span>·</span>
          <span className="text-[#ece8e2]">0ms daemon overhead</span>
        </div>
        <div className="flex items-center gap-2 text-[11px]">
          <span className="w-1.5 h-1.5 rounded-full bg-[#e17a56] animate-pulse"></span>
          <span>posix flock active</span>
        </div>
      </div>

      {/* Main minimal headline */}
      <h1 className="text-2xl sm:text-4xl font-extrabold text-[#ece8e2] font-mono tracking-tight leading-snug">
        Run the same agent CLI <br className="hidden sm:inline" />
        as many times as you want.
      </h1>

      <p className="mt-3 text-sm sm:text-base text-[#8d8983] font-mono leading-relaxed max-w-2xl">
        Each instance with its own private config, login, and proxy. No containers, no namespaces, no background daemon. Pure process execution.
      </p>

      {/* Interactive Floating Process Scene (inspired by claudes.dev) */}
      <div className="relative h-48 sm:h-52 my-6 rounded-lg bg-[#181818]/60 border border-[#222222] overflow-hidden select-none">
        
        {/* Subtle grid dots background */}
        <div
          className="absolute inset-0 opacity-15 pointer-events-none"
          style={{
            backgroundImage: 'radial-gradient(#ece8e2 1px, transparent 1px)',
            backgroundSize: '16px 16px'
          }}
        />

        <div className="absolute top-2 left-3 text-[10px] text-[#8d8983] font-mono">
          // concurrent instance cluster (click to inspect state)
        </div>

        {/* Floating instance nodes */}
        {NODES.map((node) => {
          const isSelected = focusedNode.alias === node.alias;
          return (
            <div
              key={node.alias}
              onClick={() => {
                setFocusedNode(node);
                onSelectInstance?.(node.alias);
              }}
              style={{ animationDuration: node.dur }}
              className={`absolute ${node.x} ${node.y} transition-all duration-300 cursor-pointer animate-bounce`}
            >
              <div
                className={`p-2 sm:p-2.5 rounded text-xs font-mono transition-all backdrop-blur-sm ${
                  isSelected
                    ? 'bg-[#1e1e1e] border-2 border-[#e17a56] text-[#ece8e2] shadow-[0_0_12px_rgba(225,122,86,0.3)]'
                    : 'bg-[#181818]/90 border border-[#2c2c2c] text-[#8d8983] hover:text-[#ece8e2] hover:border-[#444]'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <span
                    className={`w-2 h-2 rounded-full ${
                      node.status === 'running'
                        ? 'bg-[#e17a56]'
                        : node.status === 'locked'
                        ? 'bg-[#ff7a66]'
                        : 'bg-[#8d8983]'
                    }`}
                  />
                  <span className="font-bold text-[#ece8e2]">{node.alias}</span>
                  <span className="text-[10px] opacity-70">({node.agent})</span>
                </div>
                <div className="text-[10px] text-[#8d8983] mt-0.5 flex items-center gap-1">
                  <span>port: {node.proxy.split(':')[1] || 'direct'}</span>
                  <span>·</span>
                  <span className="text-[#e17a56]">{node.flock}</span>
                </div>
              </div>
            </div>
          );
        })}

        {/* Focused Inspector Drawer at bottom of scene */}
        <div className="absolute bottom-0 inset-x-0 bg-[#141414]/95 border-t border-[#222222] p-2.5 px-4 flex flex-col sm:flex-row sm:items-center justify-between text-xs text-[#8d8983] font-mono">
          <div className="flex items-center gap-3">
            <span className="text-[#ece8e2] font-bold">~/.golunch/instances/{focusedNode.alias}/</span>
            <span className="hidden sm:inline">·</span>
            <span>agent: <strong className="text-[#ece8e2]">{focusedNode.agent}</strong></span>
            <span className="hidden sm:inline">·</span>
            <span>proxy: <code className="text-[#e17a56]">{focusedNode.proxy}</code></span>
          </div>
          <div className="text-[11px] text-[#8d8983] mt-1 sm:mt-0">
            status: <span className="text-[#ece8e2]">{focusedNode.status}</span> (kernel flock: {focusedNode.flock})
          </div>
        </div>
      </div>

      {/* Minimal Action Row */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-3">
        {/* Copy command bar */}
        <div
          onClick={copyInstall}
          className="flex-1 flex items-center justify-between px-3 py-2 rounded bg-[#181818] border border-[#242424] hover:border-[#383838] transition-colors cursor-pointer group text-xs font-mono text-[#ece8e2]"
        >
          <div className="flex items-center gap-2 truncate">
            <span className="text-[#e17a56] font-bold select-none">$</span>
            <span className="truncate">go install github.com/raaaas/golunch/cmd/golunch@latest</span>
          </div>
          <div className="flex items-center gap-1 text-[#8d8983] group-hover:text-[#ece8e2] shrink-0 ml-2">
            {copied ? (
              <>
                <Check className="w-3.5 h-3.5 text-[#e17a56]" />
                <span className="text-[#e17a56] text-[11px]">copied</span>
              </>
            ) : (
              <>
                <Copy className="w-3.5 h-3.5" />
                <span className="text-[11px]">copy</span>
              </>
            )}
          </div>
        </div>

        {/* Quick link */}
        <a
          href="https://github.com/raaaas/golunch"
          target="_blank"
          rel="noopener noreferrer"
          className="px-4 py-2 rounded bg-[#ece8e2] hover:bg-white text-[#141414] font-bold text-xs font-mono transition-colors text-center shrink-0 cursor-pointer"
        >
          view on github →
        </a>
      </div>

    </div>
  );
};
