import { useState } from 'react';
import { Play, RotateCcw, Check, AlertCircle, Clock } from 'lucide-react';

interface Task {
  id: string;
  inst: string;
  prompt: string;
  status: 'idle' | 'running' | 'ok' | 'fail';
  result?: string;
  dur?: string;
}

const TASKS_INIT: Task[] = [
  { id: 't1', inst: 'work', prompt: 'verify OAuth token refresh logic', status: 'idle' },
  { id: 't2', inst: 'work', prompt: 'scan SQL migration for NULL safety', status: 'idle' },
  { id: 't3', inst: 'staging', prompt: 'experimental ast-lint (fails gracefully)', status: 'idle' },
  { id: 't4', inst: 'evals', prompt: 'benchmark event serializer', status: 'idle' },
  { id: 't5', inst: 'work', prompt: 'export markdown doc sync', status: 'idle' },
];

export const MinimalTaskSweep = () => {
  const [tasks, setTasks] = useState<Task[]>(TASKS_INIT);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [parallel, setParallel] = useState<number>(3);

  const runSweep = () => {
    setIsRunning(true);
    setTasks(TASKS_INIT.map((t) => ({ ...t, status: 'idle' })));

    // Worker 1 & 2 start
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, i) => (i < 2 ? { ...t, status: 'running' } : t))
      );
    }, 150);

    // Worker 3 starts
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t, i) => (i === 2 ? { ...t, status: 'running' } : t))
      );
    }, 350);

    // t1 done, t4 starts
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t) =>
          t.id === 't1'
            ? { ...t, status: 'ok', result: 'tests pass', dur: '1.2s' }
            : t.id === 't4'
            ? { ...t, status: 'running' }
            : t
        )
      );
    }, 1200);

    // t3 fails (non-fatal!)
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t) =>
          t.id === 't3'
            ? { ...t, status: 'fail', result: 'syntax err (exit 1, sweep continues)', dur: '1.4s' }
            : t.id === 't5'
            ? { ...t, status: 'running' }
            : t
        )
      );
    }, 1700);

    // t2 done
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t) =>
          t.id === 't2'
            ? { ...t, status: 'ok', result: 'migration clean', dur: '1.9s' }
            : t
        )
      );
    }, 2000);

    // t4 & t5 done
    setTimeout(() => {
      setTasks((prev) =>
        prev.map((t) =>
          t.id === 't4'
            ? { ...t, status: 'ok', result: '100k events/s', dur: '2.1s' }
            : t.id === 't5'
            ? { ...t, status: 'ok', result: '14 commands synced', dur: '1.1s' }
            : t
        )
      );
      setIsRunning(false);
    }, 2600);
  };

  const doneCount = tasks.filter((t) => t.status === 'ok').length;
  const failCount = tasks.filter((t) => t.status === 'fail').length;

  return (
    <div className="py-8 font-mono text-xs text-[#ece8e2]">
      
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
        <div>
          <h2 className="text-lg font-bold text-[#ece8e2]">
            // multi-instance task fan-out
          </h2>
          <p className="text-[#8d8983] text-xs mt-0.5">
            <code className="text-[#e17a56]">golunch task sweep.json -parallel {parallel}</code>
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={runSweep}
            disabled={isRunning}
            className="px-3 py-1.5 rounded bg-[#e17a56] hover:bg-[#d66e4a] text-[#141414] font-bold text-xs flex items-center gap-1.5 cursor-pointer disabled:opacity-50 transition-colors"
          >
            <Play className="w-3 h-3 fill-[#141414]" />
            <span>{isRunning ? 'running...' : 'run sweep'}</span>
          </button>

          <button
            onClick={() => {
              setTasks(TASKS_INIT);
              setIsRunning(false);
            }}
            disabled={isRunning}
            className="p-1.5 rounded bg-[#181818] border border-[#242424] text-[#8d8983] hover:text-[#ece8e2] cursor-pointer disabled:opacity-50"
          >
            <RotateCcw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Task Rows */}
      <div className="rounded-lg bg-[#181818]/60 border border-[#242424] divide-y divide-[#222] overflow-hidden">
        {tasks.map((task) => (
          <div
            key={task.id}
            className="p-2.5 px-3 flex flex-col sm:flex-row sm:items-center justify-between gap-2 hover:bg-[#1c1c1c]/50 transition-colors"
          >
            <div className="flex items-center gap-2.5">
              <span className="w-4 flex items-center justify-center shrink-0">
                {task.status === 'ok' && <Check className="w-3.5 h-3.5 text-[#e17a56]" />}
                {task.status === 'fail' && <AlertCircle className="w-3.5 h-3.5 text-[#ff7a66]" />}
                {task.status === 'running' && (
                  <span className="w-2 h-2 rounded-full bg-[#e17a56] animate-pulse" />
                )}
                {task.status === 'idle' && <Clock className="w-3 h-3 text-[#555]" />}
              </span>

              <span className="text-[#8d8983] w-12 shrink-0">[{task.inst}]</span>
              <span className="text-[#ece8e2] truncate max-w-sm sm:max-w-md">
                &quot;{task.prompt}&quot;
              </span>
            </div>

            <div className="flex items-center gap-3 text-[11px] text-[#8d8983] sm:justify-end">
              {task.result && (
                <span className={task.status === 'fail' ? 'text-[#ff7a66]' : 'text-[#8d8983]'}>
                  {task.result}
                </span>
              )}
              {task.dur && <span className="text-[#555]">{task.dur}</span>}
            </div>
          </div>
        ))}
      </div>

      {/* Sweep metrics callout */}
      <div className="mt-3 flex items-center justify-between text-[11px] text-[#8d8983] px-1">
        <div>
          progress: <span className="text-[#ece8e2]">{doneCount + failCount}/{tasks.length}</span> · 
          isolated failures: <span className={failCount > 0 ? 'text-[#ff7a66]' : 'text-[#ece8e2]'}>{failCount}</span>
        </div>
        <div className="text-[#555]">
          A failing prompt continues the remaining 49.
        </div>
      </div>

    </div>
  );
};
