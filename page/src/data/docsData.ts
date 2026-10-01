import { DocSection } from '../types.ts';

export const DOCS_SECTIONS: DocSection[] = [
  {
    id: 'overview',
    title: 'Overview & Philosophy',
    category: 'Getting Started',
    readTime: '3 min read',
    summary: 'Why golunch exists, how it differs from containerization, and what it guarantees.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'What is golunch?'
      },
      {
        type: 'paragraph',
        text: 'golunch is an ultra-lightweight execution harness for AI agent CLIs (such as Cline, Kilo, and OpenCode). It allows developers and automated pipelines to run the same agent binary as many times as needed — each instance with its own private configuration, its own session login, and its own proxy routing — without Docker, without Linux namespaces, without bind mounts, and without background daemons.'
      },
      {
        type: 'callout',
        variant: 'info',
        title: 'Core Guarantee',
        text: 'An instance is simply a directory tree on disk plus a precisely curated set of environment variables. The isolation is real because the agent is told, truthfully, where its configuration and data files live.'
      },
      {
        type: 'heading',
        level: 3,
        text: 'What it does vs. what it does NOT do'
      },
      {
        type: 'table',
        headers: ['What golunch Does', 'What golunch Does NOT Do'],
        rows: [
          ['Isolated config, data, cache, state, runtime, tmp per instance', 'No proxy server: proxy support is env-var injection only'],
          ['Unlimited instances of the same binary, each its own identity', 'No traffic interception, no base-URL rewriting, no MITM'],
          ['Headless prompt runs with normalized streaming events (NDJSON)', 'No namespaces, bind mounts, or container virtualization'],
          ['Task fan-out across instances with per-task timeouts', 'No daemon, no self-update of the tool itself, no bulky GUI'],
          ['flock-based busy protection that kernel-releases on death', 'Not a secret manager: credentials stay where the agent puts them'],
          ['seed copies MCP servers, skills, and plugins on request', 'No filesystem chroot: an absolute host path still reaches the host']
        ]
      }
    ]
  },
  {
    id: 'installation',
    title: 'Installation & Setup',
    category: 'Getting Started',
    readTime: '2 min read',
    summary: 'Install golunch using Go, prebuilt release binaries, or compile from source.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'System Requirements'
      },
      {
        type: 'list',
        items: [
          'Go 1.27 or newer (if installing via go install or building from source)',
          'Linux (x86_64, aarch64) or macOS (Apple Silicon, Intel)',
          'Non-root user: golunch explicitly refuses uid 0 to protect filesystem ownership integrity',
          'Make sure $(go env GOPATH)/bin and ~/.local/bin are both present in your shell PATH'
        ]
      },
      {
        type: 'heading',
        level: 2,
        text: 'Method 1: Go Install (Recommended)'
      },
      {
        type: 'code',
        language: 'bash',
        title: 'Terminal',
        code: `$ go install github.com/raaaas/golunch/cmd/golunch@latest
$ golunch version
$ golunch config --init   # writes ~/.golunch/config.toml`
      },
      {
        type: 'heading',
        level: 2,
        text: 'Method 2: Building from Checkout'
      },
      {
        type: 'paragraph',
        text: 'To embed the exact git describe string instead of a module pseudo-version, build with -ldflags:'
      },
      {
        type: 'code',
        language: 'bash',
        title: 'Terminal',
        code: `$ git clone https://github.com/raaaas/golunch.git
$ cd golunch
$ ver=$(git -C . describe --tags --always 2>/dev/null || echo dev)
$ go build -ldflags "-X main.version=$ver" -o ~/.local/bin/golunch ./cmd/golunch`
      },
      {
        type: 'heading',
        level: 3,
        text: 'Verifying PATH Setup'
      },
      {
        type: 'paragraph',
        text: 'Ensure ~/.local/bin is in your PATH. When an instance is created (e.g. golunch new work), golunch symlinks ~/.local/bin/work -> ~/.golunch/instances/work/launcher. This allows you to type "work" directly in any shell.'
      }
    ]
  },
  {
    id: 'architecture',
    title: 'Architecture & Isolation Engine',
    category: 'Core Concepts',
    readTime: '5 min read',
    summary: 'Detailed mechanics of BuildEnv, POSIX flock locking, and filesystem layout.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'The Instance Tree Layout'
      },
      {
        type: 'paragraph',
        text: 'All state lives cleanly inside ~/.golunch (or the location specified by the GOLUNCH_ROOT environment variable):'
      },
      {
        type: 'code',
        language: 'text',
        title: '~/.golunch directory layout',
        code: `~/.golunch/
├── config.toml                      # Global defaults and named proxy profiles
├── instances/
│   ├── .<alias>.lock                # Advisory flock, kernel-released on process death
│   └── <alias>/
│       ├── bin/                     # First on the instance PATH; holds downloaded agent
│       ├── home/                    # Redirected $HOME
│       ├── config/                  # XDG_CONFIG_HOME
│       ├── cache/                   # XDG_CACHE_HOME
│       ├── data/                    # XDG_DATA_HOME
│       ├── state/                   # XDG_STATE_HOME
│       ├── runtime/                 # XDG_RUNTIME_DIR
│       ├── tmp/                     # TMPDIR
│       ├── logs/                    # run transcripts (.ndjson) and proxy audit logs (.txt)
│       ├── launcher                 # Generated bash script for instant execution
│       └── metadata.toml            # Instance agent type, proxy config, and metadata
└── ~/.local/bin/<alias>             # Symlink pointing to .../instances/<alias>/launcher`
      },
      {
        type: 'heading',
        level: 2,
        text: 'One Environment Builder (Zero Drift)'
      },
      {
        type: 'paragraph',
        text: 'In golunch, the CLI run command, the shell command, the task batch runner, and the generated launcher script all render from the exact same internal function: BuildEnv. Unit tests assert parity in both directions. This guarantees that typing "work" in your terminal produces the exact same environment as "golunch run work --". Ambient shell environment variables are deliberately not baked into permanent scripts.'
      },
      {
        type: 'heading',
        level: 2,
        text: 'POSIX flock: Crash-Proof Concurrency'
      },
      {
        type: 'paragraph',
        text: 'Concurrent execution is governed by operating system advisory flock locks on ~/.golunch/instances/.<alias>.lock:'
      },
      {
        type: 'list',
        items: [
          'Shared lock (LOCK_SH): Acquired by normal agent runs and prompt tasks, allowing multiple concurrent read runs if supported.',
          'Exclusive lock (LOCK_EX): Acquired during administrative mutations (install, seed, rm, and --continue without --session).',
          'Automatic cleanup: Because flock is tied to the kernel file table, a crash, power outage, or SIGKILL automatically releases the lock immediately with zero stale lock files left behind.'
        ]
      },
      {
        type: 'heading',
        level: 3,
        text: 'Nesting Protection'
      },
      {
        type: 'paragraph',
        text: 'If $HOME points inside another isolated instance, golunch detects this via the system passwd database, stores data under the real home, and prints a warning. It also strips WARREN_* and GOLUNCH_INSTANCE* variables from child environments so nested agent processes never mistake themselves for their parent.'
      }
    ]
  },
  {
    id: 'proxy-engine',
    title: 'Proxy Precedence & The 8-Variable Rule',
    category: 'Core Concepts',
    readTime: '4 min read',
    summary: 'The deterministic 4-tier proxy hierarchy, case duplication, and loopback safety floor.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'Strict 4-Tier Precedence'
      },
      {
        type: 'paragraph',
        text: 'When launching any agent process or downloading dependencies, golunch resolves proxy configuration according to this deterministic hierarchy (highest priority wins):'
      },
      {
        type: 'code',
        language: 'text',
        title: 'Precedence Hierarchy',
        code: `1. CLI Flag:         --proxy / --noproxy passed directly to command
2. Instance Config:  [proxy] profile specified in ~/.golunch/instances/<alias>/metadata.toml
3. Global Default:   defaults.proxy profile configured in ~/.golunch/config.toml
4. Host Environment: Ambient system environment variables, untouched`
      },
      {
        type: 'heading',
        level: 2,
        text: 'The 8-Variable Lower/Uppercase Injection'
      },
      {
        type: 'paragraph',
        text: 'Many CLI tools, Node.js libraries, and Go runtimes behave unpredictably when proxy variables are mismatched or partially defined (e.g. HTTP_PROXY is set but http_proxy is missing). golunch deletes all 8 variables first, then writes both cases symmetrically:'
      },
      {
        type: 'list',
        items: [
          'HTTP_PROXY and http_proxy',
          'HTTPS_PROXY and https_proxy',
          'ALL_PROXY and all_proxy (configured if socks proxy is present)',
          'NO_PROXY and no_proxy'
        ]
      },
      {
        type: 'callout',
        variant: 'tip',
        title: 'Force Unsetting with "none"',
        text: 'Setting proxy = "none" explicitly unsets and wipes all eight proxy environment variables. This is the only reliable way to guarantee that an instance runs direct traffic without inheriting proxy settings from the host machine.'
      },
      {
        type: 'heading',
        level: 3,
        text: 'Loopback Floor Protection'
      },
      {
        type: 'paragraph',
        text: 'NO_PROXY always includes a hardcoded loopback floor: localhost, 127.0.0.1, 127.0.0.0/8, ::1, .localhost, 169.254.169.254, and the machine hostname. If an agent communicates with a local tool, MCP daemon, or Ollama server, traffic is never accidentally directed into a corporate tunnel.'
      }
    ]
  },
  {
    id: 'mcp-and-skills',
    title: 'MCP Servers, Skills & Plugins',
    category: 'Agent Management',
    readTime: '4 min read',
    summary: 'Copy host tools and prompts into isolated instances without exposing secrets.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'The Seeding Paradox'
      },
      {
        type: 'paragraph',
        text: 'Redirecting HOME and XDG base directories makes an instance start completely logged out — but it also means the instance initially sees none of your MCP servers, skills, or plugins, because agents store them in those same directories.'
      },
      {
        type: 'paragraph',
        text: 'golunch seed solves this by reading the agent driver registry and copying the relevant tool configurations group-by-group, with automatic credential stripping:'
      },
      {
        type: 'code',
        language: 'bash',
        title: 'Terminal',
        code: `$ golunch seed work --all
  absent  opencode.jsonc, agents, skills (host has none)
  copy    ~/.config/opencode/opencode.json -> config/opencode/opencode.json
  tree    ~/.config/opencode/skill -> config/opencode/skill (7 files)
  tree    ~/.config/opencode/plugin -> config/opencode/plugin (5 files)
seeded work (35 files copied)
warn: config/opencode/opencode.json holds apiKey; written 0600
warn: opencode.json references /home/you/.config/opencode/scripts/tool.py`
      },
      {
        type: 'heading',
        level: 2,
        text: 'Credential Refusal Rules'
      },
      {
        type: 'list',
        items: [
          'Known secret files (e.g. cline providers.json containing plaintext API keys) are refused by name and never copied.',
          'If a copied configuration file contains an apiKey-like field, permissions are automatically hardened to 0600 and a visible warning is printed.',
          'node_modules (deps group) is 50MB+ on average and is excluded by default unless explicitly requested with --with-deps.'
        ]
      },
      {
        type: 'heading',
        level: 3,
        text: 'Absolute Host Path Warning'
      },
      {
        type: 'paragraph',
        text: 'If a seeded config contains an absolute path (/home/user/...), that binary will be invoked on the host. To keep an MCP server completely local to the instance, place its executable inside <instance>/bin/ and reference it without an absolute path.'
      }
    ]
  },
  {
    id: 'task-fanout',
    title: 'Task Fan-Out & Batch Automation',
    category: 'Workflows',
    readTime: '4 min read',
    summary: 'Running parallel prompt sweeps across multiple instances with structured results.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'Taskfile Specification'
      },
      {
        type: 'paragraph',
        text: 'golunch task reads a declarative JSON configuration and distributes prompt executions across isolated instances concurrently:'
      },
      {
        type: 'code',
        language: 'json',
        title: 'sweep.json',
        code: `{
  "name": "benchmark-sweep",
  "defaults": {
    "instance": "work",
    "model": "ollama/qwen2.5",
    "parallel": 3,
    "timeout": "120s"
  },
  "tasks": [
    { "id": "arithmetic", "prompt": "What is 17 plus 25?" },
    { "prompts": ["summarize src/main.go", "check memory leaks", "run benchmarks"] },
    { "id": "slow-eval", "prompt": "deep audit", "instance": "research", "timeout": 300 }
  ]
}`
      },
      {
        type: 'heading',
        level: 2,
        text: 'Fault Tolerance & Resilience'
      },
      {
        type: 'paragraph',
        text: 'A failure in one prompt does not abort the batch. If prompt #3 fails with an error or times out, golunch records the error, preserves the exit code, and continues processing the remaining tasks. RAM ring buffers hold recent events for fast status inspection while the full transcript is safely appended to logs/run-*.ndjson.'
      }
    ]
  },
  {
    id: 'go-library',
    title: 'Using golunch as a Go Library',
    category: 'Developer SDK',
    readTime: '4 min read',
    summary: 'Directly import the golunch runner into your own Go applications and microservices.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'Pure Library Design'
      },
      {
        type: 'paragraph',
        text: 'The root golunch Go package exposes the exact same runner that powers the CLI, without needing to shell out to os.Args. NewRunner reads no environment variables and opens no descriptors of its own: all streams and configuration are injected by the caller.'
      },
      {
        type: 'code',
        language: 'go',
        title: 'main.go',
        code: `package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/raaaas/golunch"
    "github.com/raaaas/golunch/agent"
    "github.com/raaaas/golunch/config"
    "github.com/raaaas/golunch/instance"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        panic(err)
    }

    r := golunch.NewRunner(cfg, os.Stdin, os.Stdout, os.Stderr, instance.HostFromOs())

    res, err := r.Prompt(context.Background(), golunch.PromptOptions{
        Alias:   "work",
        Prompt:  "what tests are failing?",
        Timeout: 5 * time.Minute,
        OnEvent: func(ev agent.Event) bool {
            fmt.Printf("[%s] %s\\n", ev.Type, ev.Text)
            return true // return false to abort the run early
        },
    })

    switch golunch.ErrKind(err) {
    case golunch.KindBusy:
        fmt.Println("Instance is currently locked by another run.")
    case golunch.KindTimeout:
        fmt.Println("Run exceeded timeout duration.")
    case golunch.KindNotFound:
        fmt.Println("Target alias does not exist.")
    }
}`
      }
    ]
  },
  {
    id: 'diagnostics-and-quirks',
    title: 'Diagnostics, Doctor & Agent Quirks',
    category: 'Operations',
    readTime: '3 min read',
    summary: 'Auditing integrity, exit codes, and known vendor-specific agent quirks.',
    content: [
      {
        type: 'heading',
        level: 2,
        text: 'Standardized Exit Codes'
      },
      {
        type: 'table',
        headers: ['Code', 'Meaning', 'Resolution'],
        rows: [
          ['0', 'Success', 'Command completed normally'],
          ['1', 'General Failure', 'Agent or launcher error; check logs/run-*.ndjson'],
          ['2', 'Usage Error', 'Invalid flags or missing required argument'],
          ['3', 'Instance Busy', 'Another process holds the advisory flock on this instance'],
          ['4', 'Unknown Alias / Agent', 'Instance does not exist or requested driver is unregistered'],
          ['124', 'Execution Timeout', 'Process exceeded configured --timeout limit']
        ]
      },
      {
        type: 'heading',
        level: 2,
        text: 'Measured Agent Quirks'
      },
      {
        type: 'list',
        items: [
          'cline auto-approval: Cline does not accept a -y flag; auto-approval must be passed as --auto-approve <boolean>.',
          'kilo flat JSON: Kilo emits flat records ({type, sessionID, part}) rather than wrapped events. golunch handles both transparently.',
          'Database size: Kilo keeps credentials and conversation history in kilo.db which can grow to 1.2+ GB. golunch ls avoids deep directory walking unless -l is given.'
        ]
      }
    ]
  }
];
