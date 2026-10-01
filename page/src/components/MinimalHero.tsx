import { useState, useEffect } from 'react';
import { Terminal, Copy, Check, ArrowRight, Shield, Zap, Lock, Play, Plus, RefreshCw, Send, CheckCircle2 } from 'lucide-react';

interface WorkerNode {
  id: string;
  name: string;
  agent: 'cline' | 'kilo' | 'opencode';
  proxy: string;
  status: 'idle' | 'executing' | 'done';
  currentTask: string | null;
  jobsCompleted: number;
  lastTokens: number;
}

const INITIAL_NODES: WorkerNode[] = [
  {
    id: 'node-1',
    name: 'node-1',
    agent: 'cline',
    proxy: ':7890',
    status: 'idle',
    currentTask: null,
    jobsCompleted: 6,
    lastTokens: 4120
  },
  {
    id: 'node-2',
    name: 'node-2',
    agent: 'kilo',
    proxy: 'direct',
    status: 'idle',
    currentTask: null,
    jobsCompleted: 4,
    lastTokens: 8940
  },
  {
    id: 'node-3',
    name: 'node-3',
    agent: 'opencode',
    proxy: 'corp',
    status: 'idle',
    currentTask: null,
    jobsCompleted: 3,
    lastTokens: 2850
  }
];

const TASK_QUEUE = [
  { title: 'audit oauth token renewal', targetIdx: 0 },
  { title: 'benchmark event stream serialization', targetIdx: 1 },
  { title: 'scan sql migration for null safety', targetIdx: 2 },
  { title: 'seed mcp servers & block plaintext keys', targetIdx: 0 },
  { title: 'verify posix flock on kernel sigkill', targetIdx: 1 },
  { title: 'refactor proxy precedence layer', targetIdx: 2 }
];

interface MinimalHeroProps {
  onSelectInstance?: (alias: string) => void;
}

export const MinimalHero = ({ onSelectInstance }: MinimalHeroProps) => {
  const [copied, setCopied] = useState(false);
  const [nodes, setNodes] = useState<WorkerNode[]>(INITIAL_NODES);
  const [focusedNodeId, setFocusedNodeId] = useState<string>('node-1');

  // Flow State:
  // idle -> 1_requesting (sender sends request to core)
  //      -> 2_dispatching  (core dispatches to selected target node)
  //      -> 3_executing    (target node runs task)
  const [flowStep, setFlowStep] = useState<'idle' | '1_requesting' | '2_dispatching' | '3_executing'>('idle');
  const [taskIndex, setTaskIndex] = useState(0);
  const [activeTask, setActiveTask] = useState(TASK_QUEUE[0]);
  const [autoLoop, setAutoLoop] = useState(true);

  const copyInstall = () => {
    navigator.clipboard.writeText('go install github.com/raaaas/golunch/cmd/golunch@latest');
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  // Main lifecycle: Node sends request -> Core dispatches to one of the 3 nodes
  const dispatchNextTask = () => {
    if (flowStep !== 'idle') return;

    const task = TASK_QUEUE[taskIndex % TASK_QUEUE.length];
    setActiveTask(task);
    setTaskIndex((i) => i + 1);

    // Step 1: Sender Node sends request to Core
    setFlowStep('1_requesting');

    setTimeout(() => {
      // Step 2: Core receives request and dispatches to target of the 3 nodes
      setFlowStep('2_dispatching');

      setNodes((prev) =>
        prev.map((n, idx) =>
          idx === task.targetIdx
            ? { ...n, status: 'executing', currentTask: task.title }
            : n
        )
      );

      setTimeout(() => {
        // Step 3: Target node executes the task
        setFlowStep('3_executing');

        setTimeout(() => {
          // Finished: increment completed count and return to idle
          setNodes((prev) =>
            prev.map((n, idx) =>
              idx === task.targetIdx
                ? {
                    ...n,
                    status: 'done',
                    currentTask: null,
                    jobsCompleted: n.jobsCompleted + 1,
                    lastTokens: Math.floor(Math.random() * 5000) + 3200
                  }
                : n
            )
          );
          setFlowStep('idle');
        }, 1400);
      }, 700);
    }, 800);
  };

  // Auto-run loop
  useEffect(() => {
    let timer: NodeJS.Timeout;
    if (autoLoop && flowStep === 'idle') {
      timer = setTimeout(() => {
        dispatchNextTask();
      }, 2600);
    }
    return () => clearTimeout(timer);
  }, [autoLoop, flowStep, taskIndex]);

  const focusedNode = nodes.find((n) => n.id === focusedNodeId) || nodes[0];
  const targetNode = nodes[activeTask.targetIdx];

  return (
    <div className="pt-8 pb-8 sm:pt-12 sm:pb-12 text-left">
      
      {/* Top minimal status bar */}
      <div className="flex items-center justify-between text-xs text-[#8d8983] mb-4 pb-2 border-b border-[#222222]">
        <div className="flex items-center gap-2">
          <span className="text-[#e17a56] font-bold">GOLUNCH</span>
          <span>·</span>
          <span>v1.0.0</span>
          <span>·</span>
          <span className="text-[#ece8e2]">Go Orchestration Dispatcher</span>
        </div>
        <div className="flex items-center gap-2 text-[11px]">
          <span className="w-1.5 h-1.5 rounded-full bg-[#e17a56]"></span>
          <span>POSIX flock active</span>
        </div>
      </div>

      {/* Main minimal headline */}
      <h1 className="text-2xl sm:text-4xl font-extrabold text-[#ece8e2] font-mono tracking-tight leading-snug">
        Node requests Core. <br className="hidden sm:inline" />
        Core dispatches across 3 nodes.
      </h1>

      <p className="mt-3 text-sm sm:text-base text-[#8d8983] font-mono leading-relaxed max-w-2xl">
        A client node sends tasks into the central Go harness. Core validates advisory locks, builds the isolated environments, and multiplexes requests to the 3 target worker nodes.
      </p>

      {/* ============================================================== */}
      {/* INTERACTIVE SCENE: SENDER (LEFT) -> CORE (CENTER) -> 3 NODES (RIGHT) */}
      {/* ============================================================== */}
      <div className="relative my-6 rounded-lg bg-[#141414] border border-[#242424] overflow-hidden select-none flex flex-col justify-between shadow-2xl">
        
        {/* Subtle grid background */}
        <div
          className="absolute inset-0 opacity-10 pointer-events-none"
          style={{
            backgroundImage: 'radial-gradient(#ece8e2 1px, transparent 1px)',
            backgroundSize: '20px 20px'
          }}
        />

        {/* Scene Header */}
        <div className="relative z-10 px-4 py-2.5 bg-[#181818]/90 border-b border-[#222222] flex flex-wrap items-center justify-between gap-3 text-xs font-mono">
          
          <div className="flex items-center gap-2 text-[11px]">
            <span
              className={`px-2 py-0.5 rounded transition-colors ${
                flowStep === '1_requesting'
                  ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-500/50 font-bold'
                  : 'text-[#666]'
              }`}
            >
              1. request-node → Core
            </span>
            <span className="text-[#444]">→</span>
            <span
              className={`px-2 py-0.5 rounded transition-colors ${
                flowStep === '2_dispatching' || flowStep === '3_executing'
                  ? 'bg-[#e17a56]/20 text-[#e17a56] border border-[#e17a56]/50 font-bold'
                  : 'text-[#666]'
              }`}
            >
              2. Core dispatches → {targetNode.name}
            </span>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={dispatchNextTask}
              disabled={flowStep !== 'idle'}
              className="px-3 py-1 rounded bg-[#e17a56] hover:bg-[#d66e4a] text-[#141414] font-bold text-xs flex items-center gap-1.5 cursor-pointer disabled:opacity-50 transition-all active:scale-95"
            >
              <Send className="w-3 h-3" />
              <span>Send Request</span>
            </button>

            <button
              onClick={() => setAutoLoop(!autoLoop)}
              className={`px-2.5 py-1 rounded transition-colors text-[11px] cursor-pointer flex items-center gap-1 ${
                autoLoop
                  ? 'bg-[#e17a56]/15 text-[#e17a56] border border-[#e17a56]/40'
                  : 'bg-[#181818] text-[#8d8983] border border-[#242424] hover:text-[#ece8e2]'
              }`}
            >
              <RefreshCw className={`w-3 h-3 ${autoLoop ? 'animate-spin' : ''}`} />
              <span>{autoLoop ? 'Loop: ON' : 'Loop: OFF'}</span>
            </button>
          </div>

        </div>

        {/* ============================================================== */}
        {/* DIAGRAM AREA: SENDER (LEFT) -> CORE (CENTER) -> 3 NODES (RIGHT) */}
        {/* ============================================================== */}
        <div className="relative min-h-[300px] sm:min-h-[340px] w-full p-4 sm:p-6 flex items-center justify-between">
          
          {/* Animated SVG Conduit Lines */}
          <svg className="absolute inset-0 w-full h-full pointer-events-none" xmlns="http://www.w3.org/2000/svg">
            {/* Left: Sender to Core */}
            <line
              x1="22%"
              y1="50%"
              x2="48%"
              y2="50%"
              stroke={flowStep === '1_requesting' ? '#38bdf8' : '#262626'}
              strokeWidth={flowStep === '1_requesting' ? 2 : 1}
              strokeDasharray={flowStep === '1_requesting' ? '4 2' : '2 2'}
            />

            {/* Right: Core to Node 1 (Top) */}
            <line
              x1="52%"
              y1="50%"
              x2="78%"
              y2="20%"
              stroke={
                activeTask.targetIdx === 0 && (flowStep === '2_dispatching' || flowStep === '3_executing')
                  ? '#e17a56'
                  : '#262626'
              }
              strokeWidth={activeTask.targetIdx === 0 && flowStep === '2_dispatching' ? 2 : 1}
              strokeDasharray="3 3"
            />

            {/* Right: Core to Node 2 (Middle) */}
            <line
              x1="52%"
              y1="50%"
              x2="78%"
              y2="50%"
              stroke={
                activeTask.targetIdx === 1 && (flowStep === '2_dispatching' || flowStep === '3_executing')
                  ? '#e17a56'
                  : '#262626'
              }
              strokeWidth={activeTask.targetIdx === 1 && flowStep === '2_dispatching' ? 2 : 1}
              strokeDasharray="3 3"
            />

            {/* Right: Core to Node 3 (Bottom) */}
            <line
              x1="52%"
              y1="50%"
              x2="78%"
              y2="80%"
              stroke={
                activeTask.targetIdx === 2 && (flowStep === '2_dispatching' || flowStep === '3_executing')
                  ? '#e17a56'
                  : '#262626'
              }
              strokeWidth={activeTask.targetIdx === 2 && flowStep === '2_dispatching' ? 2 : 1}
              strokeDasharray="3 3"
            />
          </svg>

          {/* ------------------------------------------------------------ */}
          {/* ZONE 1: SENDER NODE (LEFT)                                    */}
          {/* ------------------------------------------------------------ */}
          <div className="relative z-10 w-44 sm:w-48">
            <div
              className={`p-3 rounded-lg border font-mono text-xs transition-all ${
                flowStep === '1_requesting'
                  ? 'bg-cyan-950/40 border-cyan-400 text-[#ece8e2] shadow-[0_0_12px_rgba(56,189,248,0.25)]'
                  : 'bg-[#181818]/90 border-[#2a2a2a] text-[#8d8983]'
              }`}
            >
              <div className="flex items-center justify-between pb-1.5 mb-1.5 border-b border-[#262626]">
                <div className="flex items-center gap-1.5">
                  <span
                    className={`w-2 h-2 rounded-full ${
                      flowStep === '1_requesting' ? 'bg-cyan-400' : 'bg-[#555]'
                    }`}
                  />
                  <span className="font-bold text-[#ece8e2]">request-node</span>
                </div>
                <span className="text-[10px] text-cyan-400">client</span>
              </div>

              <div className="text-[11px] text-[#8d8983] space-y-1">
                <div>action: <span className="text-[#ece8e2]">dispatch_task</span></div>
                <div className="text-[10px] text-[#ffb08a] bg-[#141414] p-1 rounded border border-[#222] truncate">
                  &quot;{activeTask.title}&quot;
                </div>
              </div>

              {flowStep === '1_requesting' && (
                <div className="mt-2 text-[10px] text-cyan-300 font-bold flex items-center gap-1">
                  <span>sending to core</span>
                  <span>→</span>
                </div>
              )}
            </div>
          </div>

          {/* ------------------------------------------------------------ */}
          {/* ZONE 2: CORE HARNESS (CENTER)                                 */}
          {/* ------------------------------------------------------------ */}
          <div className="relative z-20 flex flex-col items-center">
            <div
              className={`w-24 h-24 sm:w-28 sm:h-28 rounded-full bg-[#181818] border-2 transition-colors flex flex-col items-center justify-center p-2 text-center ${
                flowStep === '1_requesting'
                  ? 'border-cyan-400'
                  : flowStep === '2_dispatching' || flowStep === '3_executing'
                  ? 'border-[#e17a56]'
                  : 'border-[#333333]'
              }`}
            >
              <span className="text-[9px] uppercase tracking-wider text-[#e17a56] font-bold">
                CORE HARNESS
              </span>
              <span className="text-sm sm:text-base font-extrabold text-[#ece8e2] font-mono my-0.5 flex items-center gap-1">
                Go
                <span className="w-1.5 h-1.5 rounded-full bg-[#e17a56]"></span>
              </span>
              <span className="text-[9px] text-[#8d8983] font-mono leading-tight">
                {flowStep === '1_requesting'
                  ? 'receiving req'
                  : flowStep === '2_dispatching'
                  ? `routing → ${targetNode.name}`
                  : flowStep === '3_executing'
                  ? 'monitoring'
                  : 'listening'}
              </span>
            </div>

            {/* Core Payload Badge */}
            <div className="mt-2 text-center">
              {flowStep === '2_dispatching' && (
                <div className="px-2.5 py-0.5 rounded bg-[#e17a56]/20 border border-[#e17a56] text-[10px] text-[#ffb08a] font-mono">
                  → dispatching to {targetNode.name}
                </div>
              )}
              {flowStep === 'idle' && (
                <div className="text-[10px] text-[#666] font-mono">
                  ready for request
                </div>
              )}
            </div>
          </div>

          {/* ------------------------------------------------------------ */}
          {/* ZONE 3: THE 3 NODES (RIGHT)                                   */}
          {/* ------------------------------------------------------------ */}
          <div className="relative z-10 w-48 sm:w-56 space-y-2">
            {nodes.map((node, idx) => {
              const isTargetActive =
                idx === activeTask.targetIdx &&
                (flowStep === '2_dispatching' || flowStep === '3_executing');
              const isFocused = focusedNodeId === node.id;

              return (
                <div
                  key={node.id}
                  onClick={() => {
                    setFocusedNodeId(node.id);
                    onSelectInstance?.(node.name);
                  }}
                  className={`p-2.5 rounded-lg border font-mono text-xs transition-all cursor-pointer ${
                    isTargetActive
                      ? 'bg-[#1e1e1e] border-2 border-[#e17a56] text-[#ece8e2] shadow-[0_0_12px_rgba(225,122,86,0.3)]'
                      : isFocused
                      ? 'bg-[#1a1a1a] border border-[#ffb08a] text-[#ece8e2]'
                      : 'bg-[#181818]/90 border-[#2a2a2a] text-[#8d8983] hover:border-[#444]'
                  }`}
                >
                  <div className="flex items-center justify-between pb-1 mb-1 border-b border-[#242424]">
                    <div className="flex items-center gap-1.5">
                      <span
                        className={`w-2 h-2 rounded-full ${
                          isTargetActive
                            ? 'bg-[#e17a56]'
                            : node.status === 'done'
                            ? 'bg-emerald-400'
                            : 'bg-[#555]'
                        }`}
                      />
                      <span className="font-bold text-[#ece8e2]">{node.name}</span>
                    </div>
                    <span className="text-[10px] text-[#e17a56]">({node.agent})</span>
                  </div>

                  <div className="text-[10px] text-[#8d8983] space-y-0.5">
                    <div className="flex items-center justify-between">
                      <span>proxy: {node.proxy}</span>
                      <span>jobs: <strong className="text-[#ece8e2]">{node.jobsCompleted}</strong></span>
                    </div>

                    {isTargetActive ? (
                      <div className="text-[10px] text-[#ffb08a] font-semibold truncate pt-0.5 border-t border-[#262626]">
                        running: &quot;{activeTask.title}&quot;
                      </div>
                    ) : (
                      <div className="text-[9px] text-[#666] truncate pt-0.5 border-t border-[#262626]">
                        ~/.golunch/instances/{node.name}/
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>

        </div>

        {/* Bottom Inspector Bar */}
        <div className="relative z-10 bg-[#161616] px-4 py-2 border-t border-[#222222] flex flex-col sm:flex-row sm:items-center justify-between text-xs text-[#8d8983] font-mono">
          <div className="flex items-center gap-2 truncate">
            <span className="text-[#e17a56] font-bold">Selected Instance:</span>
            <span className="text-[#ece8e2] font-semibold">
              ~/.golunch/instances/{focusedNode.name}/
            </span>
            <span className="text-[#8d8983]">({focusedNode.agent} · proxy {focusedNode.proxy})</span>
          </div>
          <div className="text-[11px] text-[#8d8983] mt-1 sm:mt-0 flex items-center gap-2">
            <span>jobs: <strong className="text-[#ffb08a]">{focusedNode.jobsCompleted}</strong></span>
            <span>·</span>
            <span>isolation: <strong className="text-[#ece8e2]">POSIX flock</strong></span>
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
