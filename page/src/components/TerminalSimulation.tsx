import { useState, useEffect, useRef } from 'react';
import { Play, RotateCcw, Copy, Check, Terminal as TerminalIcon } from 'lucide-react';

interface Step {
  id: number;
  label: string;
  command: string;
  comment?: string;
  lines: {
    type: 'input' | 'output' | 'success' | 'warn' | 'jsonl' | 'comment';
    text: string;
    json?: {
      type: string;
      agent: string;
      tool?: { name: string; input: Record<string, unknown> };
      text?: string;
      usage?: { input_tokens: number; output_tokens: number; cost: number };
    };
  }[];
}

const STEPS: Step[] = [
  {
    id: 0,
    label: '1. Create Instance',
    command: 'golunch new work --agent cline --proxy http://127.0.0.1:7890',
    comment: 'Instant isolation: private HOME, XDG dirs, and launcher symlink',
    lines: [
      { type: 'input', text: 'golunch new work --agent cline --proxy http://127.0.0.1:7890' },
      { type: 'comment', text: '# Created instance at ~/.golunch/instances/work' },
      { type: 'output', text: '  instance: ~/.golunch/instances/work' },
      { type: 'output', text: '  launcher: ~/.golunch/instances/work/launcher' },
      { type: 'output', text: '  link:     ~/.local/bin/work -> launcher' },
      { type: 'output', text: '  proxy:    http://127.0.0.1:7890 (8 env vars configured)' },
      { type: 'success', text: '✓ instance "work" initialized (agent: cline)' }
    ]
  },
  {
    id: 1,
    label: '2. Verify Isolation',
    command: 'golunch doctor work',
    comment: 'Isolation proven: agent starts logged out and lock is verified',
    lines: [
      { type: 'input', text: 'golunch doctor work' },
      { type: 'comment', text: '# Checking kernel advisory locks and auth isolation' },
      { type: 'success', text: '✓ isolation: clean (instance is logged out — auth isolated)' },
      { type: 'success', text: '✓ binary: /usr/local/bin/cline (executable by current user)' },
      { type: 'success', text: '✓ launcher: in sync with BuildEnv (0 drift)' },
      { type: 'success', text: '✓ flock: available (flock ~/.golunch/instances/.work.lock)' },
      { type: 'output', text: 'All checks passed. Run "work" or "golunch run work".' }
    ]
  },
  {
    id: 2,
    label: '3. Seed MCP & Skills',
    command: 'golunch seed work --all',
    comment: 'Brings host MCP servers and skills in, while refusing passwords & API keys',
    lines: [
      { type: 'input', text: 'golunch seed work --all' },
      { type: 'output', text: '  copy  ~/.config/cline/settings.json -> config/cline/settings.json' },
      { type: 'output', text: '  tree  ~/.cline/skills -> home/.cline/skills (6 files)' },
      { type: 'output', text: '  tree  ~/.cline/mcp -> config/cline/mcp (4 servers)' },
      { type: 'warn', text: '  refuse ~/.cline/data/settings/providers.json (holds plaintext apiKey)' },
      { type: 'warn', text: '  warn: mcp/pg_query references host path /usr/bin/psql' },
      { type: 'success', text: '✓ seeded work (14 files synchronized, credentials protected)' }
    ]
  },
  {
    id: 3,
    label: '4. Run Headless Task',
    command: 'golunch run work --prompt "what tests are failing?" --timeout 5m --jsonl',
    comment: 'Streaming normalized NDJSON events directly to terminal or pipeline',
    lines: [
      { type: 'input', text: 'golunch run work --prompt "what tests are failing?" --jsonl' },
      {
        type: 'jsonl',
        text: '{"type":"tool_call","agent":"cline","tool":{"name":"list_files","input":{"dir":"./tests"}}}',
        json: {
          type: 'tool_call',
          agent: 'cline',
          tool: { name: 'list_files', input: { dir: './tests' } }
        }
      },
      {
        type: 'jsonl',
        text: '{"type":"text","agent":"cline","text":"Inspecting test runner outputs..."}',
        json: {
          type: 'text',
          agent: 'cline',
          text: 'Inspecting test runner outputs: found 2 failing test assertions in pkg/auth/token_test.go:42'
        }
      },
      {
        type: 'jsonl',
        text: '{"type":"usage","agent":"cline","usage":{"input_tokens":41203,"output_tokens":812,"cost":0.19}}',
        json: {
          type: 'usage',
          agent: 'cline',
          usage: { input_tokens: 41203, output_tokens: 812, cost: 0.19 }
        }
      },
      { type: 'success', text: '✓ prompt completed (exit code: 0, time: 2.8s)' }
    ]
  }
];

export const TerminalSimulation = () => {
  const [currentStepIndex, setCurrentStepIndex] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [copied, setCopied] = useState(false);
  const terminalRef = useRef<HTMLDivElement>(null);

  const step = STEPS[currentStepIndex];

  // Auto-play timer
  useEffect(() => {
    let timer: NodeJS.Timeout;
    if (isPlaying) {
      timer = setTimeout(() => {
        setCurrentStepIndex((prev) => (prev + 1) % STEPS.length);
      }, 3500);
    }
    return () => clearTimeout(timer);
  }, [isPlaying, currentStepIndex]);

  const copyCommand = () => {
    navigator.clipboard.writeText(step.command);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="w-full rounded-xl border border-neutral-800 bg-neutral-900/90 shadow-2xl overflow-hidden flex flex-col font-mono text-xs sm:text-sm">
      {/* Top Bar / Step Selector */}
      <div className="bg-neutral-950 px-4 py-2.5 border-b border-neutral-800 flex flex-wrap items-center justify-between gap-3">
        {/* Window dots */}
        <div className="flex items-center gap-2">
          <div className="w-3 h-3 rounded-full bg-rose-500/80"></div>
          <div className="w-3 h-3 rounded-full bg-amber-500/80"></div>
          <div className="w-3 h-3 rounded-full bg-emerald-500/80"></div>
          <span className="ml-2 text-xs text-neutral-400 font-sans font-medium flex items-center gap-1.5">
            <TerminalIcon className="w-3.5 h-3.5 text-neutral-500" />
            golunch terminal
          </span>
        </div>

        {/* Step buttons */}
        <div className="flex items-center gap-1 bg-neutral-900 p-0.5 rounded-lg border border-neutral-800/80 overflow-x-auto">
          {STEPS.map((s, idx) => (
            <button
              key={s.id}
              onClick={() => {
                setCurrentStepIndex(idx);
                setIsPlaying(false);
              }}
              className={`px-2.5 py-1 text-xs rounded transition-colors whitespace-nowrap cursor-pointer ${
                currentStepIndex === idx
                  ? 'bg-amber-500/15 text-amber-300 font-medium border border-amber-500/30'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => setIsPlaying(!isPlaying)}
            className={`p-1.5 rounded border text-xs flex items-center gap-1 transition-colors cursor-pointer ${
              isPlaying
                ? 'bg-amber-500/20 text-amber-300 border-amber-500/40'
                : 'bg-neutral-800 text-neutral-300 border-neutral-700 hover:bg-neutral-700'
            }`}
            title={isPlaying ? 'Pause Auto-Play' : 'Start Auto-Play'}
          >
            {isPlaying ? (
              <span className="w-3 h-3 flex items-center justify-center font-bold">||</span>
            ) : (
              <Play className="w-3 h-3" />
            )}
            <span className="hidden sm:inline font-sans text-xs">
              {isPlaying ? 'Pause' : 'Auto-Play'}
            </span>
          </button>

          <button
            onClick={() => {
              setCurrentStepIndex(0);
              setIsPlaying(false);
            }}
            className="p-1.5 rounded border border-neutral-800 bg-neutral-900 text-neutral-400 hover:text-neutral-200 cursor-pointer"
            title="Reset to step 1"
          >
            <RotateCcw className="w-3 h-3" />
          </button>
        </div>
      </div>

      {/* Terminal Viewport */}
      <div
        ref={terminalRef}
        className="p-4 sm:p-6 bg-neutral-950/95 min-h-[300px] max-h-[380px] overflow-y-auto space-y-2 leading-relaxed"
      >
        <div className="text-neutral-500 text-xs italic mb-2">
          # {step.comment}
        </div>

        {step.lines.map((line, idx) => {
          if (line.type === 'input') {
            return (
              <div key={idx} className="flex items-center justify-between text-neutral-100 font-semibold group pt-1">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="text-emerald-400 select-none">$</span>
                  <span className="text-amber-300">{line.text.split(' ')[0]}</span>
                  <span>{line.text.slice(line.text.indexOf(' '))}</span>
                </div>
                <button
                  onClick={copyCommand}
                  className="opacity-0 group-hover:opacity-100 p-1 text-neutral-400 hover:text-white transition-opacity cursor-pointer"
                  title="Copy command"
                >
                  {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
            );
          }

          if (line.type === 'jsonl' && line.json) {
            return (
              <div
                key={idx}
                className="my-2 p-2.5 rounded-lg border border-neutral-800 bg-neutral-900/60 font-mono text-xs space-y-1.5"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider bg-neutral-800 text-neutral-300 border border-neutral-700">
                      {line.json.type}
                    </span>
                    <span className="text-neutral-400">agent: <strong className="text-neutral-200">{line.json.agent}</strong></span>
                  </div>
                  {line.json.usage && (
                    <span className="text-[11px] text-amber-400 tabular-nums">
                      ${line.json.usage.cost} · {line.json.usage.input_tokens.toLocaleString()} tokens
                    </span>
                  )}
                </div>

                {line.json.tool && (
                  <div className="text-cyan-300 bg-cyan-950/30 p-1.5 rounded border border-cyan-800/40 text-[11px]">
                    tool: <span className="font-bold">{line.json.tool.name}</span>({JSON.stringify(line.json.tool.input)})
                  </div>
                )}

                {line.json.text && (
                  <div className="text-neutral-300 text-xs">
                    {line.json.text}
                  </div>
                )}
              </div>
            );
          }

          if (line.type === 'success') {
            return (
              <div key={idx} className="text-emerald-400 font-medium">
                {line.text}
              </div>
            );
          }

          if (line.type === 'warn') {
            return (
              <div key={idx} className="text-amber-400/90 font-medium">
                {line.text}
              </div>
            );
          }

          if (line.type === 'comment') {
            return (
              <div key={idx} className="text-neutral-500 italic">
                {line.text}
              </div>
            );
          }

          return (
            <div key={idx} className="text-neutral-400">
              {line.text}
            </div>
          );
        })}

        {/* Blinking cursor */}
        <div className="flex items-center gap-2 pt-2">
          <span className="text-emerald-400 select-none">$</span>
          <span className="w-2 h-4 bg-amber-400/80 animate-pulse inline-block"></span>
        </div>
      </div>

      {/* Terminal bottom bar */}
      <div className="bg-neutral-950/80 px-4 py-2 border-t border-neutral-800/80 flex items-center justify-between text-xs text-neutral-400 font-sans">
        <div className="flex items-center gap-3">
          <span>Target: <strong className="text-neutral-200">work</strong> (~/.golunch/instances/work)</span>
          <span className="hidden sm:inline">·</span>
          <span className="hidden sm:inline text-neutral-500">Lock: <code className="text-emerald-400 font-mono">LOCK_SH (flock)</code></span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-neutral-500">Step {currentStepIndex + 1} of {STEPS.length}</span>
        </div>
      </div>
    </div>
  );
};
