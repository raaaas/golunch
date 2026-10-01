import { useState } from 'react';
import { ShieldCheck, ShieldAlert, FileText, Check, AlertTriangle, ArrowRight, Lock } from 'lucide-react';

interface SeedItem {
  id: string;
  source: string;
  target: string;
  type: 'mcp' | 'skills' | 'credential' | 'config';
  status: 'allowed' | 'blocked' | 'hardened';
  description: string;
  warning?: string;
}

const SEED_RULES: SeedItem[] = [
  {
    id: 'mcp-settings',
    source: '~/.config/opencode/opencode.json',
    target: 'config/opencode/opencode.json',
    type: 'mcp',
    status: 'allowed',
    description: 'Host MCP server definitions and connection parameters.'
  },
  {
    id: 'cline-secret',
    source: '~/.cline/data/settings/providers.json',
    target: 'REFUSED (Not copied)',
    type: 'credential',
    status: 'blocked',
    description: 'Holds plaintext apiKey and accessToken. Blocked by name by the agent driver registry.'
  },
  {
    id: 'skills-tree',
    source: '~/.cline/skills/*',
    target: 'home/.cline/skills/*',
    type: 'skills',
    status: 'allowed',
    description: 'Custom community skills and prompt workflows. Copied safely into private tree.'
  },
  {
    id: 'hardened-key',
    source: '~/.config/tool/custom_config.json',
    target: 'config/tool/custom_config.json',
    type: 'config',
    status: 'hardened',
    description: 'Configuration file containing an apiKey-like token.',
    warning: 'chmod 0600 applied automatically; warning printed to terminal.'
  },
  {
    id: 'node-deps',
    source: 'node_modules/ (58 MB)',
    target: 'EXCLUDED BY DEFAULT',
    type: 'config',
    status: 'blocked',
    description: 'Heavy npm dependencies omitted by default. Opt-in via --with-deps.',
    warning: 'Prevents massive tree inflation. Seed with --with-deps only if npm plugins required.'
  }
];

export const McpSeedingVisualizer = () => {
  const [selectedFilter, setSelectedFilter] = useState<'all' | 'mcp' | 'credentials' | 'skills'>('all');

  const filtered = SEED_RULES.filter((item) => {
    if (selectedFilter === 'all') return true;
    if (selectedFilter === 'mcp') return item.type === 'mcp';
    if (selectedFilter === 'credentials') return item.type === 'credential' || item.status === 'hardened';
    if (selectedFilter === 'skills') return item.type === 'skills';
    return true;
  });

  return (
    <section className="py-20 border-t border-neutral-800/80 bg-neutral-950/60">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        
        {/* Header */}
        <div className="max-w-3xl mb-12">
          <div className="text-xs font-mono text-amber-400 uppercase tracking-wider mb-2">
            Credential Protection
          </div>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white font-display tracking-tight">
            Safe Tool Seeding: <code className="text-amber-300 font-mono">golunch seed</code>
          </h2>
          <p className="mt-4 text-neutral-300 text-base leading-relaxed">
            Starting an agent logged out ensures real isolation, but isolates it from your MCP servers and custom skills. <code className="text-amber-300 font-mono">golunch seed</code> synchronizes tools safely while refusing credentials.
          </p>
        </div>

        {/* Filter buttons */}
        <div className="flex items-center gap-2 mb-6 bg-neutral-900/80 p-1 rounded-lg border border-neutral-800 w-fit">
          <button
            onClick={() => setSelectedFilter('all')}
            className={`px-3 py-1.5 text-xs font-mono rounded transition-colors cursor-pointer ${
              selectedFilter === 'all' ? 'bg-amber-500 text-neutral-950 font-bold' : 'text-neutral-400 hover:text-white'
            }`}
          >
            All Sync Rules
          </button>
          <button
            onClick={() => setSelectedFilter('credentials')}
            className={`px-3 py-1.5 text-xs font-mono rounded transition-colors cursor-pointer ${
              selectedFilter === 'credentials' ? 'bg-amber-500 text-neutral-950 font-bold' : 'text-neutral-400 hover:text-white'
            }`}
          >
            Credentials & Secrets
          </button>
          <button
            onClick={() => setSelectedFilter('mcp')}
            className={`px-3 py-1.5 text-xs font-mono rounded transition-colors cursor-pointer ${
              selectedFilter === 'mcp' ? 'bg-amber-500 text-neutral-950 font-bold' : 'text-neutral-400 hover:text-white'
            }`}
          >
            MCP Servers
          </button>
          <button
            onClick={() => setSelectedFilter('skills')}
            className={`px-3 py-1.5 text-xs font-mono rounded transition-colors cursor-pointer ${
              selectedFilter === 'skills' ? 'bg-amber-500 text-neutral-950 font-bold' : 'text-neutral-400 hover:text-white'
            }`}
          >
            Skills & Plugins
          </button>
        </div>

        {/* Rules Table */}
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 overflow-hidden shadow-xl">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs font-mono">
              <thead className="bg-neutral-950/80 border-b border-neutral-800 text-neutral-400">
                <tr>
                  <th className="py-3 px-4 font-semibold">Source Path (Host)</th>
                  <th className="py-3 px-4 font-semibold">Target (Instance)</th>
                  <th className="py-3 px-4 font-semibold">Security Action</th>
                  <th className="py-3 px-4 font-semibold font-sans">Description</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-800/80 text-neutral-300">
                {filtered.map((item) => (
                  <tr key={item.id} className="hover:bg-neutral-800/30 transition-colors">
                    <td className="py-3.5 px-4 font-semibold text-white whitespace-nowrap">
                      {item.source}
                    </td>
                    <td className="py-3.5 px-4 text-neutral-400 whitespace-nowrap">
                      {item.target}
                    </td>
                    <td className="py-3.5 px-4 whitespace-nowrap">
                      {item.status === 'blocked' ? (
                        <span className="inline-flex items-center gap-1.5 text-rose-400 font-semibold">
                          <ShieldAlert className="w-3.5 h-3.5" />
                          <span>BLOCKED</span>
                        </span>
                      ) : item.status === 'hardened' ? (
                        <span className="inline-flex items-center gap-1.5 text-amber-400 font-semibold">
                          <Lock className="w-3.5 h-3.5" />
                          <span>HARDENED 0600</span>
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 text-emerald-400 font-semibold">
                          <Check className="w-3.5 h-3.5" />
                          <span>COPIED</span>
                        </span>
                      )}
                    </td>
                    <td className="py-3.5 px-4 font-sans text-neutral-300 text-xs leading-relaxed min-w-[260px]">
                      <div>{item.description}</div>
                      {item.warning && (
                        <div className="text-[11px] text-amber-400 mt-0.5 flex items-center gap-1">
                          <AlertTriangle className="w-3 h-3 shrink-0" />
                          <span>{item.warning}</span>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="bg-neutral-950 p-4 border-t border-neutral-800 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs font-sans text-neutral-400">
            <div>
              <strong className="text-neutral-200">Dry Run Available:</strong> Run{' '}
              <code className="text-amber-300 font-mono">golunch seed work --dry-run</code> to audit changes before writing.
            </div>
            <div className="text-neutral-500 font-mono text-[11px]">
              refusal by name · POSIX permissions audit
            </div>
          </div>
        </div>

      </div>
    </section>
  );
};
