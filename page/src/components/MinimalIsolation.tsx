import { useState } from 'react';
import { Folder, FileCode, Check, Shield } from 'lucide-react';

interface FileNode {
  path: string;
  type: 'dir' | 'file';
  note: string;
  preview: string;
}

const TREE: FileNode[] = [
  {
    path: 'launcher',
    type: 'file',
    note: 'generated bash wrapper with zero BuildEnv drift',
    preview: `#!/usr/bin/env bash
# Exact parity with golunch run
export HOME="/home/user/.golunch/instances/work/home"
export XDG_CONFIG_HOME="/home/user/.golunch/instances/work/config"
export XDG_DATA_HOME="/home/user/.golunch/instances/work/data"
export PATH="/home/user/.golunch/instances/work/bin:$PATH"

# 8 proxy variables synchronized (dual-case)
export HTTP_PROXY="http://127.0.0.1:7890"
export http_proxy="http://127.0.0.1:7890"
export NO_PROXY="localhost,127.0.0.1,::1,.localhost"

exec /home/user/.golunch/instances/work/bin/cline "$@"`
  },
  {
    path: 'metadata.toml',
    type: 'file',
    note: 'driver agent, creation timestamp, and assigned proxy',
    preview: `[instance]
alias = "work"
agent = "cline"
created_at = "2026-10-01T13:45:00Z"

[proxy]
profile = "corp"
url = "http://127.0.0.1:7890"`
  },
  {
    path: '.work.lock',
    type: 'file',
    note: 'POSIX flock: kernel cleans locks automatically on SIGKILL',
    preview: `# advisory flock file
# LOCK_SH for runs, LOCK_EX for admin actions (rm, seed, install)
# Crash-safe: kernel frees file descriptor instantly on death`
  },
  {
    path: 'bin/',
    type: 'dir',
    note: 'first on PATH: holds downloaded or wrapped agent binary',
    preview: `-rwxr-xr-x  1 user user 42M cline`
  },
  {
    path: 'home/',
    type: 'dir',
    note: 'redirected $HOME for isolated dotfiles and sessions',
    preview: `drwx------  3 user user 4096 .cline/`
  }
];

export const MinimalIsolation = () => {
  const [selectedFile, setSelectedFile] = useState<FileNode>(TREE[0]);

  return (
    <div className="py-8 font-mono text-xs text-[#ece8e2]">
      
      <div className="mb-4">
        <h2 className="text-lg font-bold text-[#ece8e2]">
          // isolation without containers
        </h2>
        <p className="text-[#8d8983] text-xs mt-0.5">
          Zero Docker daemons. Zero namespaces. Pure POSIX filesystem layout & environment variables.
        </p>
      </div>

      {/* Comparison table */}
      <div className="rounded-lg bg-[#181818]/60 border border-[#242424] divide-y divide-[#222] mb-6">
        <div className="p-3 flex flex-col sm:flex-row sm:items-center justify-between text-xs">
          <span className="text-[#ece8e2] font-semibold">startup delay</span>
          <div className="flex items-center gap-4 text-[#8d8983] mt-1 sm:mt-0">
            <span>docker: ~1,500ms</span>
            <span className="text-[#e17a56] font-bold">golunch: 0ms</span>
          </div>
        </div>

        <div className="p-3 flex flex-col sm:flex-row sm:items-center justify-between text-xs">
          <span className="text-[#ece8e2] font-semibold">memory overhead</span>
          <div className="flex items-center gap-4 text-[#8d8983] mt-1 sm:mt-0">
            <span>docker: 500MB+ per daemon</span>
            <span className="text-[#e17a56] font-bold">golunch: 0MB</span>
          </div>
        </div>

        <div className="p-3 flex flex-col sm:flex-row sm:items-center justify-between text-xs">
          <span className="text-[#ece8e2] font-semibold">crash protection</span>
          <div className="flex items-center gap-4 text-[#8d8983] mt-1 sm:mt-0">
            <span>pid lockfile drift</span>
            <span className="text-[#e17a56] font-bold">kernel POSIX flock (zero stale locks)</span>
          </div>
        </div>

        <div className="p-3 flex flex-col sm:flex-row sm:items-center justify-between text-xs">
          <span className="text-[#ece8e2] font-semibold">filesystem barrier</span>
          <div className="flex items-center gap-4 text-[#8d8983] mt-1 sm:mt-0">
            <span>heavy volume mounts</span>
            <span className="text-[#e17a56] font-bold">native host tools directly accessible</span>
          </div>
        </div>
      </div>

      {/* Filesystem Inspector */}
      <div className="rounded-lg bg-[#181818]/60 border border-[#242424] p-4 space-y-3">
        <div className="text-[11px] text-[#8d8983] flex items-center justify-between">
          <span>~/.golunch/instances/work/</span>
          <span>click to inspect</span>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-12 gap-3">
          {/* File buttons */}
          <div className="sm:col-span-4 space-y-1">
            {TREE.map((item) => (
              <button
                key={item.path}
                onClick={() => setSelectedFile(item)}
                className={`w-full text-left p-2 rounded text-xs transition-colors flex items-center justify-between cursor-pointer ${
                  selectedFile.path === item.path
                    ? 'bg-[#141414] text-[#e17a56] font-bold border border-[#e17a56]/40'
                    : 'text-[#8d8983] hover:text-[#ece8e2] hover:bg-[#141414]/50'
                }`}
              >
                <span>{item.path}</span>
                <span className="text-[10px] text-[#555]">{item.type}</span>
              </button>
            ))}
          </div>

          {/* File preview */}
          <div className="sm:col-span-8 p-3 rounded bg-[#141414] border border-[#222] font-mono text-[11px] space-y-2 overflow-x-auto">
            <div className="text-[#8d8983] pb-1 border-b border-[#222] flex items-center justify-between">
              <span>{selectedFile.path}</span>
              <span className="text-[#555]">{selectedFile.note}</span>
            </div>
            <pre className="text-[#ffb08a] leading-relaxed whitespace-pre font-mono">
              {selectedFile.preview}
            </pre>
          </div>
        </div>
      </div>

    </div>
  );
};
