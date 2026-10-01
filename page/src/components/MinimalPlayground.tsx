import { useState } from 'react';
import { Play, Copy, Check, Terminal, CornerDownRight } from 'lucide-react';
import { SUPPORTED_AGENTS } from '../data/agentsData.ts';

export const MinimalPlayground = () => {
  const [action, setAction] = useState<'run' | 'new' | 'seed' | 'doctor'>('run');
  const [agent, setAgent] = useState<'cline' | 'kilo' | 'opencode'>('cline');
  const [alias, setAlias] = useState<string>('work');
  const [prompt, setPrompt] = useState<string>('what tests are failing?');
  const [proxy, setProxy] = useState<string>('http://127.0.0.1:7890');
  const [isJsonl, setIsJsonl] = useState<boolean>(true);
  const [isDryRun, setIsDryRun] = useState<boolean>(false);
  const [copied, setCopied] = useState<boolean>(false);
  const [output, setOutput] = useState<string[]>([]);
  const [isRunning, setIsRunning] = useState<boolean>(false);

  // Command string generator
  const getCommand = () => {
    if (action === 'new') {
      return `golunch new ${alias} --agent ${agent} --proxy ${proxy}`;
    }
    if (action === 'doctor') {
      return `golunch doctor ${alias}`;
    }
    if (action === 'seed') {
      return `golunch seed ${alias} --all`;
    }
    let cmd = `golunch run ${alias} --prompt "${prompt}"`;
    if (isJsonl) cmd += ` --jsonl`;
    if (isDryRun) cmd += ` --dry-run`;
    return cmd;
  };

  const currentCmd = getCommand();

  const handleCopy = () => {
    navigator.clipboard.writeText(currentCmd);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleRun = () => {
    setIsRunning(true);
    setOutput([`$ ${currentCmd}`, 'acquiring advisory flock on ~/.golunch/instances/.' + alias + '.lock...']);

    setTimeout(() => {
      if (action === 'run') {
        if (isDryRun) {
          setOutput((prev) => [
            ...prev,
            'dry-run: resolved argv and env without spawning process',
            'HOME=/home/user/.golunch/instances/' + alias + '/home',
            'XDG_CONFIG_HOME=/home/user/.golunch/instances/' + alias + '/config',
            'HTTP_PROXY=' + proxy,
            'NO_PROXY=localhost,127.0.0.1,::1,.localhost',
            'exit 0'
          ]);
        } else {
          setOutput((prev) => [
            ...prev,
            '✓ flock acquired (LOCK_SH)',
            `{"type":"thinking","agent":"${agent}","text":"Inspecting tests in repository..."}`,
            `{"type":"tool_call","agent":"${agent}","tool":{"name":"exec","input":{"cmd":"go test ./..."}}}`,
            `{"type":"text","agent":"${agent}","text":"Found 2 failing tests in pkg/auth/token_test.go:42."}`,
            `{"type":"usage","agent":"${agent}","usage":{"tokens":31200,"cost":0.14}}`,
            '✓ process completed cleanly (exit: 0)'
          ]);
        }
      } else if (action === 'new') {
        setOutput((prev) => [
          ...prev,
          `✓ created tree ~/.golunch/instances/${alias}/`,
          `✓ generated launcher with strict BuildEnv parity`,
          `✓ linked ~/.local/bin/${alias} -> launcher`,
          `✓ injected 8 proxy variables for ${proxy}`,
          `ready: run "${alias}" or "golunch run ${alias}"`
        ]);
      } else if (action === 'seed') {
        setOutput((prev) => [
          ...prev,
          `inspecting driver registry for ${agent}...`,
          `  copy  config/${agent}/mcp_settings.json`,
          `  tree  skills/ -> home/.${agent}/skills (6 files)`,
          `  block providers.json (refused: contains plaintext apiKey)`,
          `✓ 14 tools synchronized, credentials safeguarded`
        ]);
      } else if (action === 'doctor') {
        setOutput((prev) => [
          ...prev,
          `diagnostics for [${alias}]:`,
          `  ✓ isolation: unauthenticated (no credentials leaked)`,
          `  ✓ binary:    ~/.golunch/instances/${alias}/bin/${agent} (executable)`,
          `  ✓ launcher:  0 drift`,
          `  ✓ locking:   POSIX flock operational`
        ]);
      }
      setIsRunning(false);
    }, 600);
  };

  return (
    <div className="py-8 font-mono text-xs text-[#ece8e2]">
      
      <div className="mb-4">
        <h2 className="text-lg font-bold text-[#ece8e2] flex items-center gap-2">
          <span>// interactive cli playground</span>
        </h2>
        <p className="text-[#8d8983] text-xs mt-0.5">
          Construct commands, toggle parameters, and simulate isolated execution.
        </p>
      </div>

      {/* Control Box */}
      <div className="p-4 rounded-lg bg-[#181818]/60 border border-[#242424] space-y-4">
        
        {/* Row 1: Action + Agent */}
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-1 bg-[#141414] p-1 rounded border border-[#242424]">
            {(['run', 'new', 'seed', 'doctor'] as const).map((a) => (
              <button
                key={a}
                onClick={() => {
                  setAction(a);
                  setOutput([]);
                }}
                className={`px-2.5 py-1 rounded transition-colors cursor-pointer text-xs ${
                  action === a
                    ? 'bg-[#e17a56] text-[#141414] font-bold'
                    : 'text-[#8d8983] hover:text-[#ece8e2]'
                }`}
              >
                {a}
              </button>
            ))}
          </div>

          <div className="flex items-center gap-1 bg-[#141414] p-1 rounded border border-[#242424]">
            {(['cline', 'kilo', 'opencode'] as const).map((ag) => (
              <button
                key={ag}
                onClick={() => setAgent(ag)}
                className={`px-2 py-1 rounded transition-colors cursor-pointer text-xs ${
                  agent === ag
                    ? 'bg-[#282828] text-[#ffb08a] font-semibold'
                    : 'text-[#8d8983] hover:text-[#ece8e2]'
                }`}
              >
                {ag}
              </button>
            ))}
          </div>

          <div className="flex items-center gap-1.5 ml-auto">
            <span className="text-[#8d8983]">alias:</span>
            <input
              type="text"
              value={alias}
              onChange={(e) => setAlias(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
              className="w-20 px-2 py-1 rounded bg-[#141414] border border-[#242424] text-[#ece8e2] text-xs focus:outline-none focus:border-[#e17a56]"
            />
          </div>
        </div>

        {/* Dynamic Fields for 'run' */}
        {action === 'run' && (
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-[#8d8983] shrink-0">prompt:</span>
              <input
                type="text"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                className="flex-1 px-2.5 py-1.5 rounded bg-[#141414] border border-[#242424] text-[#ece8e2] text-xs focus:outline-none focus:border-[#e17a56]"
              />
            </div>

            <div className="flex items-center gap-4 text-[11px] text-[#8d8983] pt-1">
              <label className="flex items-center gap-1.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={isJsonl}
                  onChange={(e) => setIsJsonl(e.target.checked)}
                  className="rounded border-[#333] bg-[#141414] text-[#e17a56] focus:ring-0"
                />
                <span>--jsonl (normalized ndjson stream)</span>
              </label>

              <label className="flex items-center gap-1.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={isDryRun}
                  onChange={(e) => setIsDryRun(e.target.checked)}
                  className="rounded border-[#333] bg-[#141414] text-[#e17a56] focus:ring-0"
                />
                <span>--dry-run (inspect env & argv only)</span>
              </label>
            </div>
          </div>
        )}

        {/* Command bar + Execute button */}
        <div className="pt-2 border-t border-[#222] flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-[#ece8e2] font-semibold text-xs overflow-x-auto py-1">
            <span className="text-[#e17a56]">$</span>
            <span className="truncate">{currentCmd}</span>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <button
              onClick={handleCopy}
              className="p-1.5 rounded bg-[#141414] hover:bg-[#202020] text-[#8d8983] hover:text-[#ece8e2] border border-[#282828] cursor-pointer"
              title="Copy"
            >
              {copied ? <Check className="w-3.5 h-3.5 text-[#e17a56]" /> : <Copy className="w-3.5 h-3.5" />}
            </button>

            <button
              onClick={handleRun}
              disabled={isRunning}
              className="px-3 py-1.5 rounded bg-[#e17a56] hover:bg-[#d66e4a] text-[#141414] font-bold text-xs flex items-center gap-1.5 cursor-pointer transition-colors disabled:opacity-50"
            >
              <Play className="w-3 h-3 fill-[#141414]" />
              <span>{isRunning ? 'running...' : 'simulate'}</span>
            </button>
          </div>
        </div>

      </div>

      {/* Output Console */}
      {output.length > 0 && (
        <div className="mt-3 p-3.5 rounded bg-[#141414] border border-[#242424] space-y-1 text-xs">
          {output.map((line, idx) => (
            <div
              key={idx}
              className={`${
                line.startsWith('$')
                  ? 'text-[#ffb08a] font-semibold'
                  : line.startsWith('✓')
                  ? 'text-[#e17a56]'
                  : line.startsWith('{')
                  ? 'text-[#ece8e2] opacity-90 pl-2'
                  : 'text-[#8d8983]'
              }`}
            >
              {line}
            </div>
          ))}
        </div>
      )}

    </div>
  );
};
