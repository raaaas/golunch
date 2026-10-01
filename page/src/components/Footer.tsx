import { Github, Heart, Terminal, ExternalLink } from 'lucide-react';

interface FooterProps {
  onNavigate: (tabId: string) => void;
}

export const Footer = ({ onNavigate }: FooterProps) => {
  return (
    <footer className="border-t border-neutral-800/80 bg-neutral-950 py-12 text-xs text-neutral-400 font-sans">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-8 mb-8 pb-8 border-b border-neutral-800/60">
          
          {/* Brand */}
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded bg-amber-500 flex items-center justify-center text-neutral-950 font-mono font-bold text-xs">
                gl
              </div>
              <span className="text-base font-bold text-white font-mono">
                golunch
              </span>
            </div>
            <p className="text-neutral-400 text-xs leading-relaxed max-w-xs">
              Run the same agent CLI as many times as you want — each instance with its own private config, login, and proxy. No containers, no namespaces, no daemon.
            </p>
          </div>

          {/* Navigation */}
          <div className="space-y-2">
            <div className="text-xs font-mono uppercase text-neutral-200 font-semibold tracking-wider">
              Navigation
            </div>
            <ul className="space-y-1.5">
              <li>
                <button
                  onClick={() => onNavigate('overview')}
                  className="hover:text-white transition-colors cursor-pointer"
                >
                  Overview & Philosophy
                </button>
              </li>
              <li>
                <button
                  onClick={() => onNavigate('architecture')}
                  className="hover:text-white transition-colors cursor-pointer"
                >
                  Isolation Architecture
                </button>
              </li>
              <li>
                <button
                  onClick={() => onNavigate('sandbox')}
                  className="hover:text-white transition-colors cursor-pointer"
                >
                  Interactive CLI Sandbox
                </button>
              </li>
              <li>
                <button
                  onClick={() => onNavigate('tasks')}
                  className="hover:text-white transition-colors cursor-pointer"
                >
                  Task Fan-Out Matrix
                </button>
              </li>
              <li>
                <button
                  onClick={() => onNavigate('docs')}
                  className="hover:text-white transition-colors cursor-pointer"
                >
                  Full Documentation
                </button>
              </li>
            </ul>
          </div>

          {/* Resources */}
          <div className="space-y-2">
            <div className="text-xs font-mono uppercase text-neutral-200 font-semibold tracking-wider">
              Resources
            </div>
            <ul className="space-y-1.5">
              <li>
                <a
                  href="https://github.com/raaaas/golunch"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-white transition-colors inline-flex items-center gap-1"
                >
                  <span>GitHub Repository</span>
                  <ExternalLink className="w-3 h-3 text-neutral-500" />
                </a>
              </li>
              <li>
                <a
                  href="https://github.com/sponsors/raaaas"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-pink-300 transition-colors inline-flex items-center gap-1"
                >
                  <Heart className="w-3 h-3 text-pink-400" />
                  <span>Sponsor raaaas</span>
                </a>
              </li>
              <li>
                <a
                  href="https://pkg.go.dev/github.com/raaaas/golunch"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-white transition-colors inline-flex items-center gap-1"
                >
                  <span>pkg.go.dev Package Docs</span>
                  <ExternalLink className="w-3 h-3 text-neutral-500" />
                </a>
              </li>
              <li>
                <a
                  href="https://github.com/raaaas/golunch/blob/master/LICENSE"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:text-white transition-colors inline-flex items-center gap-1"
                >
                  <span>MIT License</span>
                  <ExternalLink className="w-3 h-3 text-neutral-500" />
                </a>
              </li>
            </ul>
          </div>

          {/* Quick Command */}
          <div className="space-y-2">
            <div className="text-xs font-mono uppercase text-neutral-200 font-semibold tracking-wider">
              Install Command
            </div>
            <div className="p-2.5 rounded bg-neutral-900 border border-neutral-800 text-[11px] font-mono text-neutral-300 break-all select-all">
              go install github.com/raaaas/golunch/cmd/golunch@latest
            </div>
            <p className="text-[11px] text-neutral-500">
              Tested on Go 1.27+ on Linux & macOS (POSIX flock).
            </p>
          </div>

        </div>

        {/* Bottom copyright */}
        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 text-xs text-neutral-500">
          <div>
            © {new Date().getFullYear()} raaaas/golunch. Released under the MIT License.
          </div>
          <div className="flex items-center gap-4">
            <span>Pure process harness</span>
            <span aria-hidden="true">·</span>
            <span>No daemon</span>
            <span aria-hidden="true">·</span>
            <span>Zero containers</span>
          </div>
        </div>
      </div>
    </footer>
  );
};
