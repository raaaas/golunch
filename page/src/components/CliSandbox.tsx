import { useState } from 'react';
import { Terminal, Copy, Check, Play, RefreshCw, AlertTriangle, ShieldCheck, Sparkles } from 'lucide-react';
import { SUPPORTED_AGENTS } from '../data/agentsData.ts';
import { CLI_COMMANDS } from '../data/commandsData.ts';

export const CliSandbox = () => {
  const [selectedCmd, setSelectedCmd] = useState<string>('run');
  const [selectedAgent, setSelectedAgent] = useState<string>('cline');
  const [alias, setAlias] = useState<string>('work');
  const [prompt, setPrompt] = useState<string>('what tests are failing?');
  const [proxy, setProxy] = useState<string>('http://127.0.0.1:7890');
  const [jsonl, setJsonl] = useState<boolean>(true);
  const [dryRun, setDryRun] = useState<boolean>(false);
  const [thinking, setThinking] = useState<boolean>(false);
  const [autoApprove, setAutoApprove] = useState<boolean>(true);
  const [timeoutSec, setTimeoutSec] = useState<string>('5m');
  const [model, setModel] = useState<string>('anthropic/claude-sonnet-4-5');

  // Simulation execution state
  const [isExecuting, setIsExecuting] = useState<boolean>(false);
  const [executionOutput, setExecutionOutput] = useState<string[]>([]);
  const [copied, setCopied] = useState<boolean>(false);

  // Generate real command based on user choices
  const buildCommand = () => {
    let cmd = `golunch ${selectedCmd} ${alias}`;

    if (selectedCmd === 'new') {
      cmd += ` --agent ${selectedAgent}`;
      if (proxy && proxy !== 'none') cmd += ` --proxy ${proxy}`;
      if (proxy === 'none') cmd += ` --noproxy`;
      return cmd;
    }

    if (selectedCmd === 'run') {
      cmd += ` --prompt "${prompt}"`;
      if (model) cmd += ` -m ${model}`;
      if (timeoutSec) cmd += ` --timeout ${timeoutSec}`;
      if (jsonl) cmd += ` --jsonl`;
      if (thinking) cmd += ` --thinking`;
      if (autoApprove) cmd += ` --auto-approve`;
      if (dryRun) cmd += ` --dry-run`;
      if (proxy && proxy !== 'none') cmd += ` --proxy ${proxy}`;
      return cmd;
    }

    if (selectedCmd === 'seed') {
      cmd += ` --all`;
      if (dryRun) cmd += ` --dry-run`;
      return cmd;
    }

    if (selectedCmd === 'doctor') {
      if (jsonl) cmd += ` --json`;
      return cmd;
    }

    if (selectedCmd === 'clone') {
      cmd = `golunch clone ${alias} ${alias}-backup --proxy none`;
      return cmd;
    }

    if (selectedCmd === 'install') {
      cmd = `golunch install ${alias} --url https://agent.dev/install.sh --sha256 8a4b3c9d...`;
      return cmd;
    }

    if (selectedCmd === 'ls') {
      cmd = `golunch ls -l`;
      return cmd;
    }

    return cmd;
  };

  const currentCommand = buildCommand();

  const handleCopy = () => {
    navigator.clipboard.writeText(currentCommand);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleSimulate = () => {
    setIsExecuting(true);
    setExecutionOutput([`$ ${currentCommand}`, 'Acquiring kernel flock on ~/.golunch/instances/.' + alias + '.lock...']);

    setTimeout(() => {
      if (selectedCmd === 'new') {
        setExecutionOutput((prev) => [
          ...prev,
          '✓ Verified non-root UID (uid: 1000)',
          `✓ Created directory tree at ~/.golunch/instances/${alias}/`,
          `  bin/      ~/.golunch/instances/${alias}/bin/ (first on PATH)`,
          `  home/     ~/.golunch/instances/${alias}/home/ (HOME redirected)`,
          `  config/   ~/.golunch/instances/${alias}/config/ (XDG_CONFIG_HOME)`,
          `  launcher  ~/.golunch/instances/${alias}/launcher (BuildEnv parity verified)`,
          `  symlink   ~/.local/bin/${alias} -> launcher`,
          `  proxy     Synchronized 8 variables for ${proxy}`,
          `✓ Instance "${alias}" ready. Test with: "golunch doctor ${alias}"`
        ]);
        setIsExecuting(false);
      } else if (selectedCmd === 'run') {
        if (dryRun) {
          setExecutionOutput((prev) => [
            ...prev,
            '=== Dry Run (No process started, no lock held) ===',
            `Binary:     /usr/local/bin/${selectedAgent}`,
            `Argv:       [--prompt "${prompt}" --model "${model}"]`,
            `HOME:       /home/user/.golunch/instances/${alias}/home`,
            `XDG_CONFIG: /home/user/.golunch/instances/${alias}/config`,
            `HTTP_PROXY: ${proxy}`,
            `NO_PROXY:   localhost,127.0.0.1,::1,.localhost`,
            'Exit: 0'
          ]);
          setIsExecuting(false);
        } else {
          setExecutionOutput((prev) => [
            ...prev,
            '✓ Shared flock acquired (LOCK_SH)',
            `Spawning child process: /home/user/.golunch/instances/${alias}/bin/${selectedAgent}`,
            'Streaming NDJSON normalized events:'
          ]);

          setTimeout(() => {
            setExecutionOutput((prev) => [
              ...prev,
              '{"type":"thinking","agent":"' + selectedAgent + '","text":"Inspecting test runner outputs..."}',
              '{"type":"tool_call","agent":"' + selectedAgent + '","tool":{"name":"list_files","input":{"path":"./src"}}}',
              '{"type":"text","agent":"' + selectedAgent + '","text":"Identified 2 failed tests in auth_test.go:42 and token_test.go:108."}',
              '{"type":"usage","agent":"' + selectedAgent + '","usage":{"input_tokens":38120,"output_tokens":640,"cost":0.17}}',
              '✓ Child process exited cleanly (exit code: 0)',
              'flock automatically released by kernel.'
            ]);
            setIsExecuting(false);
          }, 900);
        }
      } else if (selectedCmd === 'seed') {
        setExecutionOutput((prev) => [
          ...prev,
          'Inspecting driver registry for agent: ' + selectedAgent,
          '  copy  ~/.config/' + selectedAgent + '/config.json -> config/' + selectedAgent + '/config.json',
          '  tree  ~/.config/' + selectedAgent + '/skills -> home/.' + selectedAgent + '/skills (6 files)',
          '  warn  refusing ~/.cline/data/settings/providers.json (holds plaintext apiKey)',
          '  info  remote MCP servers inherit instance proxy (' + proxy + ')',
          '✓ Seed complete: 18 files copied, 1 credential file blocked (exit code: 0)'
        ]);
        setIsExecuting(false);
      } else if (selectedCmd === 'doctor') {
        setExecutionOutput((prev) => [
          ...prev,
          `Diagnostic report for [${alias}]:`,
          '  ✓ Isolation: clean (instance is logged out — no credentials leaked)',
          `  ✓ Binary: ~/.golunch/instances/${alias}/bin/${selectedAgent} (exists, executable)`,
          '  ✓ Launcher: zero drift against internal BuildEnv',
          '  ✓ Locking: flock operational, not busy',
          '  ✓ Integrity: 0 warnings, 0 drift (exit code: 0)'
        ]);
        setIsExecuting(false);
      } else {
        setExecutionOutput((prev) => [
          ...prev,
          `Executed ${selectedCmd} on instance [${alias}] successfully.`,
          'Process completed with exit code: 0'
        ]);
        setIsExecuting(false);
      }
    }, 600);
  };

  const agentObj = SUPPORTED_AGENTS.find((a) => a.id === selectedAgent);

  return (
    <section id="sandbox" className="py-20 border-t border-neutral-800/80 bg-neutral-950">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        
        {/* Section Header */}
        <div className="max-w-3xl mb-12">
          <div className="text-xs font-mono text-amber-400 uppercase tracking-wider mb-2">
            Interactive Simulator
          </div>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white font-display tracking-tight">
            CLI Workbench & Command Generator
          </h2>
          <p className="mt-4 text-neutral-300 text-base leading-relaxed">
            Construct real commands, experiment with agent driver parameters, test isolation rules, and simulate execution without touching a local terminal.
          </p>
        </div>

        {/* Workbench Layout */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
          
          {/* Controls Column */}
          <div className="lg:col-span-5 bg-neutral-900/60 p-6 rounded-xl border border-neutral-800 space-y-5">
            
            {/* Command Selector Tabs */}
            <div>
              <label className="block text-xs font-mono text-neutral-400 mb-2">
                Command Verb
              </label>
              <div className="grid grid-cols-4 gap-1.5 bg-neutral-950 p-1 rounded-lg border border-neutral-800">
                {['new', 'run', 'seed', 'doctor', 'task', 'clone', 'install', 'ls'].map((cmd) => (
                  <button
                    key={cmd}
                    onClick={() => {
                      setSelectedCmd(cmd);
                      setExecutionOutput([]);
                    }}
                    className={`py-1.5 text-xs font-mono rounded transition-colors cursor-pointer ${
                      selectedCmd === cmd
                        ? 'bg-amber-500 text-neutral-950 font-bold shadow-sm'
                        : 'text-neutral-400 hover:text-white'
                    }`}
                  >
                    {cmd}
                  </button>
                ))}
              </div>
            </div>

            {/* Agent Selector */}
            <div>
              <label className="block text-xs font-mono text-neutral-400 mb-2">
                Agent Driver
              </label>
              <div className="grid grid-cols-3 gap-2">
                {SUPPORTED_AGENTS.map((agent) => (
                  <button
                    key={agent.id}
                    onClick={() => setSelectedAgent(agent.id)}
                    className={`p-2.5 rounded-lg border text-left transition-all cursor-pointer ${
                      selectedAgent === agent.id
                        ? 'bg-neutral-800 border-amber-500/60 text-white shadow-sm'
                        : 'bg-neutral-950/60 border-neutral-800 text-neutral-400 hover:border-neutral-700'
                    }`}
                  >
                    <div className="font-semibold text-xs text-white">{agent.name}</div>
                    <div className="text-[10px] text-neutral-400 truncate mt-0.5">{agent.eventSchema}</div>
                  </button>
                ))}
              </div>
            </div>

            {/* Instance Alias */}
            <div>
              <label className="block text-xs font-mono text-neutral-400 mb-1.5">
                Target Instance Alias
              </label>
              <input
                type="text"
                value={alias}
                onChange={(e) => setAlias(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
                className="w-full bg-neutral-950 border border-neutral-800 rounded-lg px-3 py-2 text-xs font-mono text-white focus:outline-none focus:border-amber-500"
                placeholder="e.g. work, research, eval"
              />
            </div>

            {/* Dynamic Options for 'run' */}
            {selectedCmd === 'run' && (
              <>
                <div>
                  <label className="block text-xs font-mono text-neutral-400 mb-1.5">
                    Prompt (--prompt)
                  </label>
                  <textarea
                    rows={2}
                    value={prompt}
                    onChange={(e) => setPrompt(e.target.value)}
                    className="w-full bg-neutral-950 border border-neutral-800 rounded-lg px-3 py-2 text-xs font-mono text-white focus:outline-none focus:border-amber-500 resize-none"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-mono text-neutral-400 mb-1">
                      Model (-m)
                    </label>
                    <select
                      value={model}
                      onChange={(e) => setModel(e.target.value)}
                      className="w-full bg-neutral-950 border border-neutral-800 rounded-lg px-2.5 py-1.5 text-xs font-mono text-white focus:outline-none focus:border-amber-500"
                    >
                      <option value="anthropic/claude-sonnet-4-5">claude-sonnet-4-5</option>
                      <option value="anthropic/claude-opus-4">claude-opus-4</option>
                      <option value="openai/gpt-4o">gpt-4o</option>
                      <option value="ollama/qwen2.5">ollama/qwen2.5</option>
                    </select>
                  </div>

                  <div>
                    <label className="block text-[11px] font-mono text-neutral-400 mb-1">
                      Timeout (--timeout)
                    </label>
                    <input
                      type="text"
                      value={timeoutSec}
                      onChange={(e) => setTimeoutSec(e.target.value)}
                      className="w-full bg-neutral-950 border border-neutral-800 rounded-lg px-2.5 py-1.5 text-xs font-mono text-white focus:outline-none focus:border-amber-500"
                      placeholder="e.g. 90s, 5m"
                    />
                  </div>
                </div>

                {/* Flags Toggles */}
                <div className="grid grid-cols-2 gap-2 pt-1">
                  <label className="flex items-center gap-2 text-xs font-mono text-neutral-300 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={jsonl}
                      onChange={(e) => setJsonl(e.target.checked)}
                      className="rounded border-neutral-700 bg-neutral-900 text-amber-500 focus:ring-0"
                    />
                    <span>--jsonl</span>
                  </label>

                  <label className="flex items-center gap-2 text-xs font-mono text-neutral-300 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={dryRun}
                      onChange={(e) => setDryRun(e.target.checked)}
                      className="rounded border-neutral-700 bg-neutral-900 text-amber-500 focus:ring-0"
                    />
                    <span>--dry-run</span>
                  </label>

                  <label className="flex items-center gap-2 text-xs font-mono text-neutral-300 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={thinking}
                      onChange={(e) => setThinking(e.target.checked)}
                      className="rounded border-neutral-700 bg-neutral-900 text-amber-500 focus:ring-0"
                    />
                    <span>--thinking</span>
                  </label>

                  <label className="flex items-center gap-2 text-xs font-mono text-neutral-300 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={autoApprove}
                      onChange={(e) => setAutoApprove(e.target.checked)}
                      className="rounded border-neutral-700 bg-neutral-900 text-amber-500 focus:ring-0"
                    />
                    <span>--auto-approve</span>
                  </label>
                </div>
              </>
            )}

            {/* Proxy Input */}
            <div>
              <label className="block text-xs font-mono text-neutral-400 mb-1.5">
                Proxy Profile or URL (--proxy)
              </label>
              <input
                type="text"
                value={proxy}
                onChange={(e) => setProxy(e.target.value)}
                className="w-full bg-neutral-950 border border-neutral-800 rounded-lg px-3 py-2 text-xs font-mono text-white focus:outline-none focus:border-amber-500"
                placeholder="http://127.0.0.1:7890, corp, or none"
              />
              <span className="text-[10px] text-neutral-500 mt-1 block">
                Type &quot;none&quot; to force-unset all 8 proxy variables.
              </span>
            </div>

            {/* Agent Quirks Badge */}
            {agentObj && (
              <div className="p-3 rounded-lg bg-neutral-950 border border-neutral-800/80 text-xs">
                <div className="font-semibold text-neutral-300 mb-1 flex items-center gap-1.5">
                  <AlertTriangle className="w-3.5 h-3.5 text-amber-400" />
                  <span>Agent Driver Rules: {agentObj.name}</span>
                </div>
                <ul className="text-neutral-400 text-[11px] space-y-1 list-disc list-inside">
                  {agentObj.quirks.map((q, idx) => (
                    <li key={idx}>{q}</li>
                  ))}
                </ul>
              </div>
            )}

          </div>

          {/* Terminal Output Column */}
          <div className="lg:col-span-7 space-y-4">
            
            {/* Live Command Banner */}
            <div className="p-4 rounded-xl border border-neutral-800 bg-neutral-900/90 shadow-xl">
              <div className="flex items-center justify-between pb-2 mb-2 border-b border-neutral-800">
                <span className="text-xs font-mono text-neutral-400 flex items-center gap-1.5">
                  <Terminal className="w-3.5 h-3.5 text-amber-400" />
                  Generated Shell Invocation
                </span>
                <div className="flex items-center gap-2">
                  <button
                    onClick={handleCopy}
                    className="p-1.5 rounded hover:bg-neutral-800 text-neutral-400 hover:text-white transition-colors cursor-pointer text-xs flex items-center gap-1"
                    title="Copy command"
                  >
                    {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    <span>{copied ? 'Copied' : 'Copy'}</span>
                  </button>
                  <button
                    onClick={handleSimulate}
                    disabled={isExecuting}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-500 hover:bg-amber-400 text-neutral-950 font-semibold text-xs transition-colors cursor-pointer active:scale-95 disabled:opacity-50"
                  >
                    {isExecuting ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <Play className="w-3.5 h-3.5 fill-neutral-950" />
                    )}
                    <span>Simulate</span>
                  </button>
                </div>
              </div>

              <div className="font-mono text-xs sm:text-sm text-neutral-100 p-2.5 rounded bg-neutral-950 border border-neutral-800/80 break-all select-all">
                <span className="text-emerald-400 select-none mr-2">$</span>
                {currentCommand}
              </div>
            </div>

            {/* Output Simulator Window */}
            <div className="rounded-xl border border-neutral-800 bg-neutral-950 shadow-2xl overflow-hidden font-mono text-xs">
              <div className="bg-neutral-900/90 px-4 py-2 border-b border-neutral-800 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="w-2.5 h-2.5 rounded-full bg-rose-500/70"></div>
                  <div className="w-2.5 h-2.5 rounded-full bg-amber-500/70"></div>
                  <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/70"></div>
                  <span className="text-[11px] text-neutral-400 ml-2">stdout / stderr stream</span>
                </div>
                {executionOutput.length > 0 && (
                  <button
                    onClick={() => setExecutionOutput([])}
                    className="text-[11px] text-neutral-500 hover:text-neutral-300"
                  >
                    Clear
                  </button>
                )}
              </div>

              <div className="p-4 sm:p-5 min-h-[260px] max-h-[340px] overflow-y-auto space-y-1.5 leading-relaxed text-neutral-300">
                {executionOutput.length === 0 ? (
                  <div className="h-48 flex flex-col items-center justify-center text-center text-neutral-500 font-sans space-y-2">
                    <Terminal className="w-8 h-8 text-neutral-600 stroke-[1.5]" />
                    <p className="text-xs">Click <strong className="text-neutral-300">&quot;Simulate&quot;</strong> above to execute the command in this sandbox.</p>
                    <p className="text-[11px] text-neutral-600 font-mono">Simulates flock lock acquisition, process launch, and stream normalization.</p>
                  </div>
                ) : (
                  executionOutput.map((line, idx) => (
                    <div
                      key={idx}
                      className={`${
                        line.startsWith('$')
                          ? 'text-white font-bold pb-1'
                          : line.startsWith('✓')
                          ? 'text-emerald-400'
                          : line.includes('warn')
                          ? 'text-amber-400'
                          : line.startsWith('{')
                          ? 'text-cyan-300 bg-cyan-950/20 p-1.5 rounded border border-cyan-800/30'
                          : 'text-neutral-400'
                      }`}
                    >
                      {line}
                    </div>
                  ))
                )}
              </div>

              <div className="bg-neutral-900/60 px-4 py-2 border-t border-neutral-800 text-[11px] text-neutral-500 flex items-center justify-between">
                <span>Kernel lock: <code className="text-neutral-400">flock(2)</code></span>
                <span>Status: <span className="text-emerald-400">{isExecuting ? 'running...' : 'ready'}</span></span>
              </div>
            </div>

          </div>

        </div>

      </div>
    </section>
  );
};
