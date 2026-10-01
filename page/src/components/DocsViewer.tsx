import { useState, useMemo } from 'react';
import { Search, BookOpen, Terminal, Copy, Check, ChevronRight, AlertCircle, Info, Lightbulb, ExternalLink } from 'lucide-react';
import { DOCS_SECTIONS } from '../data/docsData.ts';
import { CLI_COMMANDS } from '../data/commandsData.ts';
import { DocSection, DocBlock, CliCommand } from '../types.ts';

export const DocsViewer = () => {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeSectionId, setActiveSectionId] = useState<string>('overview');
  const [activeTab, setActiveTab] = useState<'guides' | 'commands'>('guides');
  const [selectedCmdName, setSelectedCmdName] = useState<string>('new');
  const [copiedCodeIndex, setCopiedCodeIndex] = useState<string | null>(null);

  // Filter sections by search query
  const filteredSections = useMemo(() => {
    if (!searchQuery.trim()) return DOCS_SECTIONS;
    const q = searchQuery.toLowerCase();
    return DOCS_SECTIONS.filter(
      (s) =>
        s.title.toLowerCase().includes(q) ||
        s.summary.toLowerCase().includes(q) ||
        s.category.toLowerCase().includes(q)
    );
  }, [searchQuery]);

  // Filter commands by search query
  const filteredCommands = useMemo(() => {
    if (!searchQuery.trim()) return CLI_COMMANDS;
    const q = searchQuery.toLowerCase();
    return CLI_COMMANDS.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.summary.toLowerCase().includes(q) ||
        c.description.toLowerCase().includes(q)
    );
  }, [searchQuery]);

  const activeSection = useMemo(() => {
    return DOCS_SECTIONS.find((s) => s.id === activeSectionId) || DOCS_SECTIONS[0];
  }, [activeSectionId]);

  const activeCommand = useMemo(() => {
    return CLI_COMMANDS.find((c) => c.name === selectedCmdName) || CLI_COMMANDS[0];
  }, [selectedCmdName]);

  const copyCode = (code: string, id: string) => {
    navigator.clipboard.writeText(code);
    setCopiedCodeIndex(id);
    setTimeout(() => setCopiedCodeIndex(null), 2000);
  };

  // Group sections by category for the sidebar
  const categories = useMemo(() => {
    const map = new Map<string, DocSection[]>();
    filteredSections.forEach((sec) => {
      const list = map.get(sec.category) || [];
      list.push(sec);
      map.set(sec.category, list);
    });
    return Array.from(map.entries());
  }, [filteredSections]);

  return (
    <section id="docs" className="py-16 border-t border-neutral-800/80 bg-neutral-950">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        
        {/* Header */}
        <div className="max-w-3xl mb-8">
          <div className="text-xs font-mono text-amber-400 uppercase tracking-wider mb-2">
            Documentation Portal
          </div>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white font-display tracking-tight">
            Complete Guides & CLI Reference
          </h2>
          <p className="mt-3 text-neutral-300 text-base leading-relaxed">
            In-depth guides on isolation internals, proxy precedence rules, MCP synchronization, and the full specification of all 14 commands.
          </p>
        </div>

        {/* Search & Mode Switcher */}
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-4 mb-8 pb-4 border-b border-neutral-800">
          
          {/* Search bar */}
          <div className="relative flex-1 max-w-md">
            <Search className="w-4 h-4 text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search concepts, commands, flags..."
              className="w-full bg-neutral-900 border border-neutral-800 rounded-lg pl-9 pr-4 py-2 text-xs font-mono text-white placeholder-neutral-500 focus:outline-none focus:border-amber-500"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-500 hover:text-white"
              >
                Clear
              </button>
            )}
          </div>

          {/* Subtabs: Guides vs Commands */}
          <div className="flex items-center gap-1 bg-neutral-900 p-1 rounded-lg border border-neutral-800">
            <button
              onClick={() => setActiveTab('guides')}
              className={`px-4 py-1.5 text-xs font-medium rounded-md transition-colors flex items-center gap-2 cursor-pointer ${
                activeTab === 'guides'
                  ? 'bg-neutral-800 text-white font-semibold'
                  : 'text-neutral-400 hover:text-white'
              }`}
            >
              <BookOpen className="w-3.5 h-3.5" />
              <span>Guides & Concepts</span>
            </button>
            <button
              onClick={() => setActiveTab('commands')}
              className={`px-4 py-1.5 text-xs font-medium rounded-md transition-colors flex items-center gap-2 cursor-pointer ${
                activeTab === 'commands'
                  ? 'bg-neutral-800 text-white font-semibold'
                  : 'text-neutral-400 hover:text-white'
              }`}
            >
              <Terminal className="w-3.5 h-3.5" />
              <span>All 14 Commands</span>
            </button>
          </div>
        </div>

        {/* Layout: Sidebar + Document Content */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
          
          {/* Sidebar */}
          <div className="lg:col-span-4 bg-neutral-900/40 p-4 rounded-xl border border-neutral-800 max-h-[750px] overflow-y-auto space-y-6">
            {activeTab === 'guides' ? (
              categories.map(([category, items]) => (
                <div key={category} className="space-y-1">
                  <div className="text-[11px] font-mono uppercase text-neutral-400 font-semibold px-2 mb-1.5 tracking-wider">
                    {category}
                  </div>
                  {items.map((item) => (
                    <button
                      key={item.id}
                      onClick={() => setActiveSectionId(item.id)}
                      className={`w-full text-left px-3 py-2 rounded-lg text-xs transition-colors flex items-center justify-between cursor-pointer ${
                        activeSectionId === item.id
                          ? 'bg-amber-500/15 text-amber-300 font-semibold border border-amber-500/30'
                          : 'text-neutral-300 hover:bg-neutral-800/60'
                      }`}
                    >
                      <span className="truncate">{item.title}</span>
                      <ChevronRight className={`w-3 h-3 shrink-0 ${activeSectionId === item.id ? 'text-amber-400' : 'text-neutral-600'}`} />
                    </button>
                  ))}
                </div>
              ))
            ) : (
              <div className="space-y-1">
                <div className="text-[11px] font-mono uppercase text-neutral-400 font-semibold px-2 mb-2 tracking-wider">
                  CLI Commands ({filteredCommands.length})
                </div>
                {filteredCommands.map((cmd) => (
                  <button
                    key={cmd.name}
                    onClick={() => setSelectedCmdName(cmd.name)}
                    className={`w-full text-left px-3 py-2 rounded-lg text-xs font-mono transition-colors flex items-center justify-between cursor-pointer ${
                      selectedCmdName === cmd.name
                        ? 'bg-amber-500/15 text-amber-300 font-bold border border-amber-500/30'
                        : 'text-neutral-300 hover:bg-neutral-800/60'
                    }`}
                  >
                    <span>golunch {cmd.name}</span>
                    <span className="text-[10px] text-neutral-500 font-sans truncate max-w-[100px]">
                      {cmd.summary.split(' ')[0]}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Main Content Area */}
          <div className="lg:col-span-8 bg-neutral-900/60 p-6 sm:p-8 rounded-xl border border-neutral-800 min-h-[600px]">
            {activeTab === 'guides' ? (
              /* Guide Article View */
              <article className="space-y-6">
                
                {/* Header metadata (unboxed text) */}
                <div className="flex items-center gap-2 text-xs text-neutral-400 font-mono pb-2 border-b border-neutral-800/80">
                  <span className="text-amber-400 font-semibold uppercase">{activeSection.category}</span>
                  <span aria-hidden="true">·</span>
                  <span>{activeSection.readTime}</span>
                </div>

                <h1 className="text-2xl sm:text-3xl font-extrabold text-white font-display tracking-tight">
                  {activeSection.title}
                </h1>

                <p className="text-sm text-neutral-300 font-normal leading-relaxed border-l-2 border-amber-500/60 pl-3">
                  {activeSection.summary}
                </p>

                {/* Blocks rendering */}
                <div className="space-y-6 pt-2">
                  {activeSection.content.map((block, idx) => {
                    if (block.type === 'heading') {
                      return block.level === 2 ? (
                        <h2 key={idx} className="text-lg sm:text-xl font-bold text-white pt-4 border-t border-neutral-800/60">
                          {block.text}
                        </h2>
                      ) : (
                        <h3 key={idx} className="text-base font-semibold text-neutral-200 pt-2">
                          {block.text}
                        </h3>
                      );
                    }

                    if (block.type === 'paragraph') {
                      return (
                        <p key={idx} className="text-sm text-neutral-300 leading-relaxed font-sans">
                          {block.text}
                        </p>
                      );
                    }

                    if (block.type === 'list') {
                      return (
                        <ul key={idx} className="space-y-2 text-sm text-neutral-300 list-disc list-inside leading-relaxed font-sans">
                          {block.items.map((it, itemIdx) => (
                            <li key={itemIdx}>{it}</li>
                          ))}
                        </ul>
                      );
                    }

                    if (block.type === 'callout') {
                      const icons = {
                        info: <Info className="w-4 h-4 text-cyan-400 shrink-0 mt-0.5" />,
                        warning: <AlertCircle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />,
                        tip: <Lightbulb className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
                      };

                      return (
                        <div key={idx} className="p-4 rounded-lg bg-neutral-950 border border-neutral-800 flex items-start gap-3 text-xs leading-relaxed">
                          {icons[block.variant]}
                          <div>
                            <div className="font-semibold text-white mb-1 font-sans">{block.title}</div>
                            <div className="text-neutral-300 font-sans">{block.text}</div>
                          </div>
                        </div>
                      );
                    }

                    if (block.type === 'code') {
                      const codeId = `code-${idx}`;
                      return (
                        <div key={idx} className="rounded-lg border border-neutral-800 bg-neutral-950 overflow-hidden font-mono text-xs">
                          {block.title && (
                            <div className="px-4 py-2 bg-neutral-900/90 border-b border-neutral-800 flex items-center justify-between text-[11px] text-neutral-400">
                              <span>{block.title}</span>
                              <button
                                onClick={() => copyCode(block.code, codeId)}
                                className="flex items-center gap-1 hover:text-white transition-colors cursor-pointer"
                              >
                                {copiedCodeIndex === codeId ? (
                                  <>
                                    <Check className="w-3 h-3 text-emerald-400" />
                                    <span className="text-emerald-400">Copied</span>
                                  </>
                                ) : (
                                  <>
                                    <Copy className="w-3 h-3" />
                                    <span>Copy</span>
                                  </>
                                )}
                              </button>
                            </div>
                          )}
                          <pre className="p-4 overflow-x-auto text-neutral-200 leading-relaxed">
                            {block.code}
                          </pre>
                        </div>
                      );
                    }

                    if (block.type === 'table') {
                      return (
                        <div key={idx} className="overflow-x-auto rounded-lg border border-neutral-800">
                          <table className="w-full text-left text-xs font-mono">
                            <thead className="bg-neutral-950 border-b border-neutral-800 text-neutral-400 font-semibold">
                              <tr>
                                {block.headers.map((h, hIdx) => (
                                  <th key={hIdx} className="py-2.5 px-4 font-semibold">{h}</th>
                                ))}
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-neutral-800 text-neutral-300 font-sans">
                              {block.rows.map((r, rIdx) => (
                                <tr key={rIdx} className="hover:bg-neutral-800/20">
                                  {r.map((cell, cIdx) => (
                                    <td key={cIdx} className="py-3 px-4 text-xs leading-relaxed">{cell}</td>
                                  ))}
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      );
                    }

                    return null;
                  })}
                </div>

              </article>
            ) : (
              /* Single Command Reference View */
              <div className="space-y-6">
                
                {/* Command Signature Banner */}
                <div>
                  <div className="flex items-center gap-2 text-xs font-mono text-amber-400 uppercase tracking-wider mb-2">
                    CLI COMMAND REFERENCE
                  </div>
                  <h1 className="text-2xl sm:text-3xl font-bold font-mono text-white">
                    golunch {activeCommand.name}
                  </h1>
                  <p className="mt-2 text-sm text-neutral-300 leading-relaxed font-sans">
                    {activeCommand.summary}
                  </p>
                </div>

                <div className="p-3 bg-neutral-950 rounded-lg border border-neutral-800 font-mono text-xs text-amber-300 flex items-center justify-between">
                  <span className="truncate">{activeCommand.signature}</span>
                  <button
                    onClick={() => copyCode(activeCommand.signature, 'sig')}
                    className="p-1 hover:text-white text-neutral-400 cursor-pointer"
                  >
                    {copiedCodeIndex === 'sig' ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>

                <p className="text-sm text-neutral-300 leading-relaxed font-sans">
                  {activeCommand.description}
                </p>

                {/* Flags Table */}
                {activeCommand.flags.length > 0 && (
                  <div className="space-y-3 pt-2">
                    <h3 className="text-sm font-bold text-white font-mono uppercase tracking-wide">
                      Flags & Parameters
                    </h3>
                    <div className="rounded-lg border border-neutral-800 overflow-hidden">
                      <table className="w-full text-left text-xs font-mono">
                        <thead className="bg-neutral-950 border-b border-neutral-800 text-neutral-400">
                          <tr>
                            <th className="py-2.5 px-3">Flag</th>
                            <th className="py-2.5 px-3">Type</th>
                            <th className="py-2.5 px-3 font-sans">Description</th>
                            <th className="py-2.5 px-3">Default</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-neutral-800 text-neutral-300">
                          {activeCommand.flags.map((f) => (
                            <tr key={f.flag} className="hover:bg-neutral-800/20">
                              <td className="py-2.5 px-3 font-semibold text-amber-300 whitespace-nowrap">{f.flag}</td>
                              <td className="py-2.5 px-3 text-neutral-400">{f.type}</td>
                              <td className="py-2.5 px-3 font-sans text-neutral-300">{f.description}</td>
                              <td className="py-2.5 px-3 text-neutral-400">{f.default || '—'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}

                {/* Usage Examples */}
                <div className="space-y-4 pt-2">
                  <h3 className="text-sm font-bold text-white font-mono uppercase tracking-wide">
                    Real Examples
                  </h3>
                  {activeCommand.examples.map((ex, idx) => (
                    <div key={idx} className="rounded-lg border border-neutral-800 bg-neutral-950 overflow-hidden text-xs font-mono">
                      <div className="px-4 py-2 bg-neutral-900/90 border-b border-neutral-800 flex items-center justify-between text-neutral-300">
                        <span className="font-semibold">{ex.title}</span>
                        <button
                          onClick={() => copyCode(ex.command, `ex-${idx}`)}
                          className="hover:text-white cursor-pointer"
                        >
                          {copiedCodeIndex === `ex-${idx}` ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                        </button>
                      </div>
                      <div className="p-3 text-emerald-400 select-all overflow-x-auto">
                        $ {ex.command}
                      </div>
                      {ex.output && (
                        <div className="p-3 bg-neutral-900/50 border-t border-neutral-800/80 text-neutral-400 whitespace-pre font-mono text-[11px] overflow-x-auto">
                          {ex.output}
                        </div>
                      )}
                      {ex.explanation && (
                        <div className="px-3 py-2 bg-neutral-900/30 border-t border-neutral-800/50 text-neutral-400 font-sans text-[11px]">
                          {ex.explanation}
                        </div>
                      )}
                    </div>
                  ))}
                </div>

                {/* Notes */}
                {activeCommand.notes && (
                  <div className="p-4 rounded-lg bg-neutral-950 border border-neutral-800 text-xs text-neutral-300 space-y-1.5">
                    <div className="font-semibold text-amber-400 font-mono">Design Invariants & Notes:</div>
                    <ul className="list-disc list-inside space-y-1 text-neutral-400 font-sans">
                      {activeCommand.notes.map((note, idx) => (
                        <li key={idx}>{note}</li>
                      ))}
                    </ul>
                  </div>
                )}

              </div>
            )}
          </div>

        </div>

      </div>
    </section>
  );
};
