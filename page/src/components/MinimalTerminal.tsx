import { useState, useEffect } from 'react';
import { Play, RotateCcw, Copy, Check, Terminal as TerminalIcon } from 'lucide-react';

interface Step {
  id: string;
  tabLabel: string;
  command: string;
  summary: string;
  lines: {
    type: 'prompt' | 'info' | 'success' | 'warn' | 'json';
    text: string;
    extra?: Record<string, unknown>;
  }[];
}

const STEPS: Step[] = [
  {
    id: 'new',
    tabLabel: '01. new',
    command: 'golunch new work --agent cline --proxy http://127.0.0.1:7890',
    summary: 'initialize isolated instance with private XDG and 8-var proxy',
    lines: [
      { type: 'prompt', text: 'golunch new work --agent cline --proxy http://127.0.0.1:7890' },
      { type: 'info', text: 'mkdir ~/.golunch/instances/work/{bin,home,config,data,logs,tmp}' },
      { type: 'info', text: 'render ~/.golunch/instances/work/launcher (BuildEnv parity verified)' },
      { type: 'info', text: 'link   ~/.local/bin/work -> launcher' },
      { type: 'warn', text: 'proxy  injected HTTP_PROXY, http_proxy, HTTPS_PROXY, https_proxy, NO_PROXY' },
      { type: 'success', text: 'ok: instance "work" created (type "work" or "golunch run work")' }
    ]
  },
  {
    id: 'doctor',
    tabLabel: '02. doctor',
    command: 'golunch doctor work',
    summary: 'verify kernel advisory flock and login isolation',
    lines: [
      { type: 'prompt', text: 'golunch doctor work' },
      { type: 'success', text: '✓ isolation: clean (logged out — zero credential leakage)' },
      { type: 'success', text: '✓ binary:    /home/user/.golunch/instances/work/bin/cline (executable)' },
      { type: 'success', text: '✓ launcher:  in sync with internal BuildEnv (0 drift)' },
      { type: 'success', text: '✓ lock:      flock ~/.golunch/instances/.work.lock available' },
      { type: 'info', text: 'all invariants verified. ready for runs.' }
    ]
  },
  {
    id: 'seed',
    tabLabel: '03. seed',
    command: 'golunch seed work --all',
    summary: 'safely sync host MCP servers & skills while blocking secrets',
    lines: [
      { type: 'prompt', text: 'golunch seed work --all' },
      { type: 'info', text: 'copy  ~/.config/cline/settings.json -> config/cline/settings.json' },
      { type: 'info', text: 'tree  ~/.cline/skills -> home/.cline/skills (6 files)' },
      { type: 'warn', text: 'block ~/.cline/data/settings/providers.json (holds plaintext apiKey)' },
      { type: 'warn', text: 'warn: config references host script /usr/local/bin/pg_mcp' },
      { type: 'success', text: 'ok: seeded 14 tools into "work", secrets shielded' }
    ]
  },
  {
    id: 'run',
    tabLabel: '04. run --jsonl',
    command: 'golunch run work --prompt "what tests are failing?" --jsonl',
    summary: 'execute headless prompt with streaming normalized NDJSON',
    lines: [
      { type: 'prompt', text: 'golunch run work --prompt "what tests are failing?" --jsonl' },
      {
        type: 'json',
        text: '{"type":"thinking","agent":"cline","text":"Scanning package test suites..."}'
      },
      {
        type: 'json',
        text: '{"type":"tool_call","agent":"cline","tool":{"name":"exec","input":{"cmd":"go test ./..."}}}'
      },
      {
        type: 'json',
        text: '{"type":"text","agent":"cline","text":"Found 2 failing assertions in pkg/proxy/precedence_test.go:88."}'
      },
      {
        type: 'json',
        text: '{"type":"usage","agent":"cline","usage":{"tokens":31420,"cost":0.14}}'
      },
      { type: 'success', text: 'ok: exit code 0 (execution time: 1.8s)' }
    ]
  },
  {
    id: 'task',
    tabLabel: '05. task',
    command: 'golunch task sweep.json -parallel 3',
    summary: 'fan N prompts out across instances with per-task fault tolerance',
    lines: [
      { type: 'prompt', text: 'golunch task sweep.json -parallel 3' },
      { type: 'info', text: '[task:1] [work]     prompt: "summarize changes" -> running' },
      { type: 'info', text: '[task:2] [evals]    prompt: "run benchmark"    -> running' },
      { type: 'info', text: '[task:3] [staging]  prompt: "failing script"   -> error (non-fatal, sweep continues)' },
      { type: 'success', text: '✓ [task:1] completed (1.2s)' },
      { type: 'success', text: '✓ [task:2] completed (2.4s)' },
      { type: 'warn', text: 'summary: 2 succeeded, 1 failed (exit 1)' }
    ]
  }
];

export const MinimalTerminal = () => {
  const [activeStepId, setActiveStepId] = useState<string>('run');
  const [copied, setCopied] = useState<boolean>(false);
  const [isPlaying, setIsPlaying] = useState<boolean>(false);

  const step = STEPS.find((s) => s.id === activeStepId) || STEPS[0];

  useEffect(() => {
    let t: NodeJS.Timeout;
    if (isPlaying) {
      t = setTimeout(() => {
        const nextIdx = (STEPS.findIndex((s) => s.id === activeStepId) + 1) % STEPS.length;
        setActiveStepId(STEPS[nextIdx].id);
      }, 3500);
    }
    return () => clearTimeout(t);
  }, [isPlaying, activeStepId]);

  const copyCommand = () => {
    navigator.clipboard.writeText(step.command);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="my-8 rounded-lg bg-[#141414] border border-[#242424] font-mono text-xs overflow-hidden shadow-lg">
      
      {/* Terminal Title Bar */}
      <div className="bg-[#181818] px-3.5 py-2 border-b border-[#242424] flex flex-wrap items-center justify-between gap-2">
        
        {/* Step tabs */}
        <div className="flex items-center gap-1 overflow-x-auto">
          {STEPS.map((s) => (
            <button
              key={s.id}
              onClick={() => {
                setActiveStepId(s.id);
                setIsPlaying(false);
              }}
              className={`px-2 py-1 rounded text-xs transition-colors cursor-pointer whitespace-nowrap ${
                activeStepId === s.id
                  ? 'bg-[#141414] text-[#e17a56] font-bold border border-[#e17a56]/40'
                  : 'text-[#8d8983] hover:text-[#ece8e2]'
              }`}
            >
              {s.tabLabel}
            </button>
          ))}
        </div>

        {/* Player controls */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => setIsPlaying(!isPlaying)}
            className={`px-2 py-1 rounded text-[11px] flex items-center gap-1 cursor-pointer transition-colors ${
              isPlaying
                ? 'bg-[#e17a56]/20 text-[#e17a56] border border-[#e17a56]/40'
                : 'text-[#8d8983] hover:text-[#ece8e2] hover:bg-[#202020]'
            }`}
          >
            {isPlaying ? 'pause' : 'auto-run'}
          </button>

          <button
            onClick={copyCommand}
            className="p-1 text-[#8d8983] hover:text-[#ece8e2] cursor-pointer"
            title="Copy command"
          >
            {copied ? <Check className="w-3.5 h-3.5 text-[#e17a56]" /> : <Copy className="w-3.5 h-3.5" />}
          </button>
        </div>

      </div>

      {/* Terminal Content Body */}
      <div className="p-4 sm:p-5 space-y-2 leading-relaxed min-h-[220px] max-h-[340px] overflow-y-auto">
        <div className="text-[#8d8983] text-[11px] mb-3">
          // {step.summary}
        </div>

        {step.lines.map((line, idx) => {
          if (line.type === 'prompt') {
            return (
              <div key={idx} className="flex items-center gap-2 text-[#ece8e2] font-semibold pt-1">
                <span className="text-[#e17a56] select-none font-bold">$</span>
                <span className="text-[#ffb08a]">{line.text.split(' ')[0]}</span>
                <span>{line.text.slice(line.text.indexOf(' '))}</span>
              </div>
            );
          }

          if (line.type === 'json') {
            return (
              <div
                key={idx}
                className="p-2 rounded bg-[#181818]/80 border border-[#222] text-[11px] text-[#ffb08a] font-mono leading-normal"
              >
                {line.text}
              </div>
            );
          }

          if (line.type === 'success') {
            return (
              <div key={idx} className="text-[#e17a56] font-medium text-xs">
                {line.text}
              </div>
            );
          }

          if (line.type === 'warn') {
            return (
              <div key={idx} className="text-[#8d8983] text-xs">
                {line.text}
              </div>
            );
          }

          return (
            <div key={idx} className="text-[#8d8983] text-xs">
              {line.text}
            </div>
          );
        })}

        {/* Pulsing prompt indicator */}
        <div className="flex items-center gap-1.5 pt-2 text-[#e17a56]">
          <span>$</span>
          <span className="w-1.5 h-3.5 bg-[#e17a56] animate-pulse inline-block"></span>
        </div>
      </div>

      {/* Minimal Footer Info */}
      <div className="bg-[#181818]/60 px-4 py-1.5 border-t border-[#222] text-[11px] text-[#8d8983] flex items-center justify-between">
        <span>flock(2) kernel-released</span>
        <span>Linux & macOS native</span>
      </div>

    </div>
  );
};
