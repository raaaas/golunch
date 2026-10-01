import { useState } from 'react';
import { Play, RotateCcw, CheckCircle2, AlertCircle, Clock, Zap, Layers, FileJson } from 'lucide-react';

interface TaskItem {
  id: string;
  instance: string;
  prompt: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  duration?: string;
  tokens?: number;
  cost?: number;
  eventSnippet?: string;
}

const INITIAL_TASKS: TaskItem[] = [
  {
    id: 'auth-spec',
    instance: 'work',
    prompt: 'Implement RFC 6749 OAuth token refresh validation',
    status: 'pending'
  },
  {
    id: 'db-migration',
    instance: 'work',
    prompt: 'Check SQL schema migration for nullable columns',
    status: 'pending'
  },
  {
    id: 'failing-lint',
    instance: 'staging',
    prompt: 'Run experimental AST linter with strict flags',
    status: 'pending'
  },
  {
    id: 'benchmark',
    instance: 'evals',
    prompt: 'Benchmark JSONL serializer against 100k events',
    status: 'pending'
  },
  {
    id: 'doc-sync',
    instance: 'work',
    prompt: 'Sync internal cli command docs with latest flags',
    status: 'pending'
  }
];

export const TaskMatrixVisualizer = () => {
  const [tasks, setTasks] = useState<TaskItem[]>(INITIAL_TASKS);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [parallelLimit, setParallelLimit] = useState<number>(3);
  const [activeTab, setActiveTab] = useState<'visual' | 'json'>('visual');

  const startSweep = () => {
    setIsRunning(true);
    setTasks(INITIAL_TASKS.map((t) => ({ ...t, status: 'pending' })));

    // Worker 1: Task 0 (work)
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 0
            ? { ...t, status: 'running', eventSnippet: 'tool_call: inspect_package("pkg/oauth")' }
            : t
        )
      );
    }, 200);

    // Worker 2: Task 1 (work)
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 1
            ? { ...t, status: 'running', eventSnippet: 'thinking: scanning schema.sql migrations...' }
            : t
        )
      );
    }, 400);

    // Worker 3: Task 2 (staging)
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 2
            ? { ...t, status: 'running', eventSnippet: 'exec: ast-lint --strict' }
            : t
        )
      );
    }, 600);

    // Finish Task 0
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 0
            ? {
                ...t,
                status: 'completed',
                duration: '1.4s',
                tokens: 4210,
                cost: 0.04,
                eventSnippet: '✓ OAuth token refresh validated'
              }
            : idx === 3
            ? { ...t, status: 'running', eventSnippet: 'tool_call: run_benchmarks()' }
            : t
        )
      );
    }, 1800);

    // Finish Task 1
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 1
            ? {
                ...t,
                status: 'completed',
                duration: '2.1s',
                tokens: 8940,
                cost: 0.08,
                eventSnippet: '✓ All migration scripts pass safety invariants'
              }
            : idx === 4
            ? { ...t, status: 'running', eventSnippet: 'tool_call: read_file("internal/cli")' }
            : t
        )
      );
    }, 2400);

    // Fail Task 2 (Graceful partial failure demonstration!)
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 2
            ? {
                ...t,
                status: 'failed',
                duration: '1.9s',
                eventSnippet: 'error: unknown AST visitor flag --strict (task failed, sweep continues)'
              }
            : t
        )
      );
    }, 2700);

    // Finish Task 3
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 3
            ? {
                ...t,
                status: 'completed',
                duration: '2.8s',
                tokens: 15300,
                cost: 0.12,
                eventSnippet: '✓ 100k events serialized in 42ms'
              }
            : t
        )
      );
    }, 3400);

    // Finish Task 4
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, idx) =>
          idx === 4
            ? {
                ...t,
                status: 'completed',
                duration: '1.7s',
                tokens: 5120,
                cost: 0.05,
                eventSnippet: '✓ 14 commands synchronized to markdown'
              }
            : t
        )
      );
      setIsRunning(false);
    }, 4200);
  };

  const resetSweep = () => {
    setTasks(INITIAL_TASKS);
    setIsRunning(false);
  };

  const completedCount = tasks.filter((t) => t.status === 'completed').length;
  const failedCount = tasks.filter((t) => t.status === 'failed').length;
  const totalTokens = tasks.reduce((sum, t) => sum + (t.tokens || 0), 0);
  const totalCost = tasks.reduce((sum, t) => sum + (t.cost || 0), 0);

  const sampleTaskfile = `{
  "name": "full-sweep",
  "defaults": {
    "instance": "work",
    "model": "anthropic/claude-sonnet-4-5",
    "parallel": ${parallelLimit},
    "timeout": "120s"
  },
  "tasks": [
    { "id": "auth-spec",    "prompt": "Implement RFC 6749 OAuth token refresh validation" },
    { "id": "db-migration", "prompt": "Check SQL schema migration for nullable columns" },
    { "id": "failing-lint", "prompt": "Run experimental AST linter with strict flags", "instance": "staging" },
    { "id": "benchmark",    "prompt": "Benchmark JSONL serializer against 100k events", "instance": "evals" },
    { "id": "doc-sync",     "prompt": "Sync internal cli command docs with latest flags" }
  ]
}`;

  return (
    <section id="tasks" className="py-20 border-t border-neutral-800/80 bg-neutral-950">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        
        {/* Section Header */}
        <div className="max-w-3xl mb-12">
          <div className="text-xs font-mono text-amber-400 uppercase tracking-wider mb-2">
            Multi-Instance Fan-Out
          </div>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white font-display tracking-tight">
            Batch Task Orchestration: <code className="text-amber-300 font-mono">golunch task</code>
          </h2>
          <p className="mt-4 text-neutral-300 text-base leading-relaxed">
            Distribute matrix sweeps across multiple isolated instances concurrently. An error or timeout in task #3 is cleanly recorded while the remaining 49 prompts continue executing without interruption.
          </p>
        </div>

        {/* Task Control Center */}
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-6 shadow-xl mb-8">
          
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-6 mb-6 border-b border-neutral-800">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-amber-500/10 border border-amber-500/20 text-amber-400">
                <Layers className="w-5 h-5" />
              </div>
              <div>
                <h3 className="text-base font-bold text-white">Parallel Prompt Dispatcher</h3>
                <p className="text-xs text-neutral-400">Executing <code className="text-neutral-300 font-mono">golunch task sweep.json -parallel {parallelLimit}</code></p>
              </div>
            </div>

            <div className="flex items-center gap-3">
              {/* Tab selector */}
              <div className="bg-neutral-950 p-1 rounded-lg border border-neutral-800 flex items-center text-xs">
                <button
                  onClick={() => setActiveTab('visual')}
                  className={`px-3 py-1 rounded transition-colors cursor-pointer ${
                    activeTab === 'visual' ? 'bg-neutral-800 text-white font-medium' : 'text-neutral-400 hover:text-neutral-200'
                  }`}
                >
                  Worker Grid
                </button>
                <button
                  onClick={() => setActiveTab('json')}
                  className={`px-3 py-1 rounded transition-colors cursor-pointer flex items-center gap-1 ${
                    activeTab === 'json' ? 'bg-neutral-800 text-white font-medium' : 'text-neutral-400 hover:text-neutral-200'
                  }`}
                >
                  <FileJson className="w-3.5 h-3.5" />
                  <span>Taskfile</span>
                </button>
              </div>

              {/* Concurrency input */}
              <div className="hidden sm:flex items-center gap-1.5 text-xs text-neutral-400 font-mono bg-neutral-950 px-2.5 py-1.5 rounded-lg border border-neutral-800">
                <span>-parallel</span>
                <select
                  value={parallelLimit}
                  onChange={(e) => setParallelLimit(Number(e.target.value))}
                  disabled={isRunning}
                  className="bg-transparent text-amber-400 font-bold focus:outline-none cursor-pointer"
                >
                  <option value={2}>2</option>
                  <option value={3}>3</option>
                  <option value={4}>4</option>
                  <option value={5}>5</option>
                </select>
              </div>

              {/* Start Sweep Button */}
              <button
                onClick={startSweep}
                disabled={isRunning}
                className="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-amber-500 hover:bg-amber-400 text-neutral-950 font-semibold text-xs transition-colors cursor-pointer active:scale-95 disabled:opacity-50"
              >
                <Play className="w-3.5 h-3.5 fill-neutral-950" />
                <span>{isRunning ? 'Sweeping...' : 'Run Sweep'}</span>
              </button>

              <button
                onClick={resetSweep}
                disabled={isRunning}
                className="p-2 rounded-lg bg-neutral-800 hover:bg-neutral-700 text-neutral-300 transition-colors cursor-pointer disabled:opacity-50"
                title="Reset sweep"
              >
                <RotateCcw className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Stats Bar */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 p-4 rounded-lg bg-neutral-950 border border-neutral-800/80 mb-6 text-xs font-mono">
            <div>
              <div className="text-neutral-500">Tasks Completed</div>
              <div className="text-base font-bold text-white tabular-nums mt-0.5">
                {completedCount} / {tasks.length}
              </div>
            </div>
            <div>
              <div className="text-neutral-500">Fault Tolerance</div>
              <div className={`text-base font-bold tabular-nums mt-0.5 ${failedCount > 0 ? 'text-rose-400' : 'text-neutral-300'}`}>
                {failedCount} isolated failure{failedCount === 1 ? '' : 's'}
              </div>
            </div>
            <div>
              <div className="text-neutral-500">Total Tokens</div>
              <div className="text-base font-bold text-amber-400 tabular-nums mt-0.5">
                {totalTokens.toLocaleString()}
              </div>
            </div>
            <div>
              <div className="text-neutral-500">Aggregate Cost</div>
              <div className="text-base font-bold text-emerald-400 tabular-nums mt-0.5">
                ${totalCost.toFixed(2)}
              </div>
            </div>
          </div>

          {/* Tab Views */}
          {activeTab === 'visual' ? (
            <div className="space-y-3">
              {tasks.map((task) => (
                <div
                  key={task.id}
                  className={`p-3.5 rounded-lg border transition-all text-xs font-mono flex flex-col md:flex-row md:items-center justify-between gap-3 ${
                    task.status === 'completed'
                      ? 'bg-neutral-950/80 border-emerald-500/40 text-neutral-200'
                      : task.status === 'failed'
                      ? 'bg-neutral-950/80 border-rose-500/40 text-neutral-200'
                      : task.status === 'running'
                      ? 'bg-amber-500/[0.04] border-amber-500/50 text-white'
                      : 'bg-neutral-950/40 border-neutral-800 text-neutral-400'
                  }`}
                >
                  <div className="flex items-start md:items-center gap-3">
                    {/* Status icon */}
                    <div className="mt-0.5 md:mt-0">
                      {task.status === 'completed' && <CheckCircle2 className="w-4 h-4 text-emerald-400" />}
                      {task.status === 'failed' && <AlertCircle className="w-4 h-4 text-rose-400" />}
                      {task.status === 'running' && <Zap className="w-4 h-4 text-amber-400 animate-pulse" />}
                      {task.status === 'pending' && <Clock className="w-4 h-4 text-neutral-600" />}
                    </div>

                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-white">{task.id}</span>
                        <span className="text-[10px] text-neutral-400 px-1.5 py-0.5 rounded bg-neutral-900 border border-neutral-800">
                          inst: {task.instance}
                        </span>
                      </div>
                      <div className="text-neutral-400 text-xs mt-0.5 font-sans">
                        &quot;{task.prompt}&quot;
                      </div>
                    </div>
                  </div>

                  {/* Output snippet */}
                  <div className="flex items-center gap-4 text-right">
                    {task.eventSnippet && (
                      <span className={`text-[11px] truncate max-w-xs md:max-w-sm ${
                        task.status === 'failed' ? 'text-rose-400' : 'text-neutral-400'
                      }`}>
                        {task.eventSnippet}
                      </span>
                    )}

                    {task.duration && (
                      <span className="text-neutral-400 text-[11px] whitespace-nowrap">
                        {task.duration}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="rounded-lg bg-neutral-950 border border-neutral-800 p-4 font-mono text-xs text-neutral-300 overflow-x-auto">
              <pre>{sampleTaskfile}</pre>
            </div>
          )}

          {/* Explanatory callout */}
          <div className="mt-6 pt-4 border-t border-neutral-800 text-xs text-neutral-400 font-sans leading-relaxed">
            <strong className="text-neutral-200">Resilience Invariant:</strong> Notice how <code className="text-rose-400 font-mono">failing-lint</code> failed without crashing the batch. golunch records the exit code, logs the full NDJSON transcript in <code className="text-neutral-300 font-mono">logs/run-*.ndjson</code>, and allows all other worker threads to complete cleanly.
          </div>

        </div>

      </div>
    </section>
  );
};
