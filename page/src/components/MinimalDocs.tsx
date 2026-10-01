import { useState, useMemo } from 'react';
import { Search, Copy, Check, ChevronRight } from 'lucide-react';
import { DOCS_SECTIONS } from '../data/docsData.ts';
import { CLI_COMMANDS } from '../data/commandsData.ts';

export const MinimalDocs = () => {
  const [search, setSearch] = useState('');
  const [activeSectionId, setActiveSectionId] = useState('overview');
  const [tab, setTab] = useState<'guides' | 'commands'>('guides');
  const [activeCmd, setActiveCmd] = useState('new');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const filteredDocs = useMemo(() => {
    if (!search.trim()) return DOCS_SECTIONS;
    const q = search.toLowerCase();
    return DOCS_SECTIONS.filter(
      (s) => s.title.toLowerCase().includes(q) || s.summary.toLowerCase().includes(q)
    );
  }, [search]);

  const filteredCmds = useMemo(() => {
    if (!search.trim()) return CLI_COMMANDS;
    const q = search.toLowerCase();
    return CLI_COMMANDS.filter(
      (c) => c.name.toLowerCase().includes(q) || c.summary.toLowerCase().includes(q)
    );
  }, [search]);

  const currentSection = DOCS_SECTIONS.find((s) => s.id === activeSectionId) || DOCS_SECTIONS[0];
  const currentCommand = CLI_COMMANDS.find((c) => c.name === activeCmd) || CLI_COMMANDS[0];

  const copy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  return (
    <div className="py-8 font-mono text-xs text-[#ece8e2]">
      
      {/* Header & Search */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-6">
        <div>
          <h2 className="text-lg font-bold text-[#ece8e2]">
            // documentation & reference
          </h2>
          <p className="text-[#8d8983] text-xs mt-0.5">
            Architecture, isolation mechanics, and complete 14-command reference.
          </p>
        </div>

        {/* Search */}
        <div className="relative">
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="search docs..."
            className="w-full sm:w-56 px-3 py-1.5 rounded bg-[#181818] border border-[#242424] text-xs text-[#ece8e2] placeholder-[#555] focus:outline-none focus:border-[#e17a56]"
          />
        </div>
      </div>

      {/* Switcher: Guides vs 14 Commands */}
      <div className="flex items-center gap-1 mb-4 border-b border-[#222] pb-2">
        <button
          onClick={() => setTab('guides')}
          className={`px-3 py-1 rounded text-xs transition-colors cursor-pointer ${
            tab === 'guides'
              ? 'bg-[#181818] text-[#e17a56] font-bold border border-[#e17a56]/30'
              : 'text-[#8d8983] hover:text-[#ece8e2]'
          }`}
        >
          guides ({filteredDocs.length})
        </button>
        <button
          onClick={() => setTab('commands')}
          className={`px-3 py-1 rounded text-xs transition-colors cursor-pointer ${
            tab === 'commands'
              ? 'bg-[#181818] text-[#e17a56] font-bold border border-[#e17a56]/30'
              : 'text-[#8d8983] hover:text-[#ece8e2]'
          }`}
        >
          all 14 commands ({filteredCmds.length})
        </button>
      </div>

      {/* Two-column layout */}
      <div className="grid grid-cols-1 md:grid-cols-12 gap-6 items-start">
        
        {/* Left selector */}
        <div className="md:col-span-4 rounded-lg bg-[#181818]/60 border border-[#242424] p-2 space-y-1 max-h-[600px] overflow-y-auto">
          {tab === 'guides' ? (
            filteredDocs.map((doc) => (
              <button
                key={doc.id}
                onClick={() => setActiveSectionId(doc.id)}
                className={`w-full text-left p-2 rounded text-xs transition-colors flex items-center justify-between cursor-pointer ${
                  activeSectionId === doc.id
                    ? 'bg-[#141414] text-[#e17a56] font-bold border border-[#e17a56]/40'
                    : 'text-[#8d8983] hover:text-[#ece8e2]'
                }`}
              >
                <span className="truncate">{doc.title}</span>
                <ChevronRight className={`w-3 h-3 shrink-0 ${activeSectionId === doc.id ? 'text-[#e17a56]' : 'text-[#444]'}`} />
              </button>
            ))
          ) : (
            filteredCmds.map((c) => (
              <button
                key={c.name}
                onClick={() => setActiveCmd(c.name)}
                className={`w-full text-left p-2 rounded text-xs transition-colors flex items-center justify-between cursor-pointer ${
                  activeCmd === c.name
                    ? 'bg-[#141414] text-[#e17a56] font-bold border border-[#e17a56]/40'
                    : 'text-[#8d8983] hover:text-[#ece8e2]'
                }`}
              >
                <span className="font-mono">golunch {c.name}</span>
                <span className="text-[10px] text-[#555] truncate max-w-[80px]">
                  {c.summary.split(' ')[0]}
                </span>
              </button>
            ))
          )}
        </div>

        {/* Right article view */}
        <div className="md:col-span-8 rounded-lg bg-[#181818]/60 border border-[#242424] p-5 sm:p-6 min-h-[450px] leading-relaxed">
          {tab === 'guides' ? (
            <div className="space-y-4">
              <div className="flex items-center justify-between pb-2 border-b border-[#222] text-[#8d8983] text-[11px]">
                <span className="text-[#e17a56]">{currentSection.category}</span>
                <span>{currentSection.readTime}</span>
              </div>

              <h3 className="text-xl font-bold text-[#ece8e2]">
                {currentSection.title}
              </h3>

              <p className="text-xs text-[#8d8983] border-l border-[#e17a56] pl-2.5">
                {currentSection.summary}
              </p>

              <div className="space-y-4 pt-2">
                {currentSection.content.map((block, idx) => {
                  if (block.type === 'heading') {
                    return (
                      <h4 key={idx} className="text-sm font-bold text-[#ece8e2] pt-2 border-t border-[#222]">
                        {block.text}
                      </h4>
                    );
                  }
                  if (block.type === 'paragraph') {
                    return (
                      <p key={idx} className="text-xs text-[#8d8983] leading-relaxed">
                        {block.text}
                      </p>
                    );
                  }
                  if (block.type === 'list') {
                    return (
                      <ul key={idx} className="space-y-1.5 text-xs text-[#8d8983] list-disc list-inside">
                        {block.items.map((it, i) => (
                          <li key={i}>{it}</li>
                        ))}
                      </ul>
                    );
                  }
                  if (block.type === 'code') {
                    const cid = `code-${idx}`;
                    return (
                      <div key={idx} className="rounded bg-[#141414] border border-[#222] overflow-hidden text-[11px]">
                        <div className="px-3 py-1.5 bg-[#181818] border-b border-[#222] flex items-center justify-between text-[#8d8983]">
                          <span>{block.title || block.language}</span>
                          <button
                            onClick={() => copy(block.code, cid)}
                            className="hover:text-[#ece8e2] cursor-pointer flex items-center gap-1"
                          >
                            {copiedId === cid ? <Check className="w-3 h-3 text-[#e17a56]" /> : <Copy className="w-3 h-3" />}
                            <span>{copiedId === cid ? 'copied' : 'copy'}</span>
                          </button>
                        </div>
                        <pre className="p-3 text-[#ffb08a] overflow-x-auto whitespace-pre">
                          {block.code}
                        </pre>
                      </div>
                    );
                  }
                  if (block.type === 'callout') {
                    return (
                      <div key={idx} className="p-3 rounded bg-[#141414] border border-[#e17a56]/30 text-xs text-[#8d8983] space-y-1">
                        <div className="text-[#e17a56] font-bold">{block.title}</div>
                        <div>{block.text}</div>
                      </div>
                    );
                  }
                  return null;
                })}
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="text-[11px] text-[#e17a56] pb-1 border-b border-[#222]">
                CLI REFERENCE
              </div>

              <h3 className="text-xl font-bold text-[#ece8e2]">
                golunch {currentCommand.name}
              </h3>

              <div className="p-2.5 rounded bg-[#141414] border border-[#222] flex items-center justify-between text-xs text-[#ffb08a]">
                <span className="truncate">{currentCommand.signature}</span>
                <button
                  onClick={() => copy(currentCommand.signature, 'sig')}
                  className="p-1 text-[#8d8983] hover:text-[#ece8e2] cursor-pointer"
                >
                  {copiedId === 'sig' ? <Check className="w-3 h-3 text-[#e17a56]" /> : <Copy className="w-3 h-3" />}
                </button>
              </div>

              <p className="text-xs text-[#8d8983] leading-relaxed">
                {currentCommand.description}
              </p>

              {currentCommand.flags.length > 0 && (
                <div className="space-y-2 pt-2">
                  <div className="text-xs font-bold text-[#ece8e2]">flags:</div>
                  <div className="space-y-1.5">
                    {currentCommand.flags.map((f) => (
                      <div key={f.flag} className="p-2 rounded bg-[#141414] border border-[#222] text-[11px] flex flex-col sm:flex-row sm:items-center justify-between gap-1">
                        <div>
                          <span className="text-[#e17a56] font-bold">{f.flag}</span>
                          <span className="text-[#555] ml-1.5">({f.type})</span>
                          <span className="text-[#8d8983] ml-2 font-sans">{f.description}</span>
                        </div>
                        {f.default && <span className="text-[#555] shrink-0">default: {f.default}</span>}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {currentCommand.examples.length > 0 && (
                <div className="space-y-2 pt-2">
                  <div className="text-xs font-bold text-[#ece8e2]">examples:</div>
                  {currentCommand.examples.map((ex, i) => (
                    <div key={i} className="p-2.5 rounded bg-[#141414] border border-[#222] space-y-1 text-[11px]">
                      <div className="text-[#8d8983]">{ex.title}</div>
                      <div className="text-[#ffb08a]">$ {ex.command}</div>
                      {ex.output && (
                        <div className="text-[#666] pt-1 border-t border-[#222] whitespace-pre text-[10px]">
                          {ex.output}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

      </div>

    </div>
  );
};
