import { CliCommand } from '../types.ts';

export const CLI_COMMANDS: CliCommand[] = [
  {
    name: 'new',
    signature: 'golunch new <alias> --agent <name> [flags]',
    summary: 'Wrap an existing host binary in a fresh isolated instance',
    description: 'Creates a new named instance directory structure (~/.golunch/instances/<alias>), configures its private XDG directories, generates the executable launcher script, and links ~/.local/bin/<alias>. Use --install to download the agent into the instance bin/ directly.',
    flags: [
      { flag: '--agent', type: 'string', description: 'Agent driver name (e.g. cline, kilo, opencode)' },
      { flag: '--install', type: 'boolean', description: 'Download the agent binary into the instance bin/ directory in one step' },
      { flag: '--proxy', type: 'string', description: 'Proxy URL or named profile from config.toml to assign to this instance' },
      { flag: '--noproxy', type: 'boolean', description: 'Force disable all proxying for this instance (unsets all 8 proxy vars)' },
      { flag: '--binary', type: 'string', description: 'Explicit path to host binary if not found in default PATH' },
      { flag: '--shell', type: 'string', description: 'Default interactive shell for this instance', default: '/bin/bash' },
      { flag: '--link', type: 'boolean', description: 'Create symlink in ~/.local/bin/<alias>', default: 'true' },
      { flag: '--force', type: 'boolean', description: 'Overwrite existing instance if alias already exists' }
    ],
    examples: [
      {
        title: 'Create instance with proxy profile',
        command: 'golunch new work --agent cline --proxy http://127.0.0.1:7890',
        explanation: 'Generates private HOME and XDG base dirs under ~/.golunch/instances/work and creates symlink ~/.local/bin/work.'
      },
      {
        title: 'Create and install agent in one step',
        command: 'golunch new dev --agent kilo --install --url https://github.com/kilo/releases/download/v1.2/install.sh',
        explanation: 'Downloads installer, verifies sha256, executes inside instance with redirected HOME, and places binary in bin/.'
      }
    ],
    notes: [
      'Refuses uid 0 (root). Root-owned instance trees leave the agent unable to write its own configuration.',
      'If $HOME is nested inside another golunch instance, resolves the real passwd home to avoid accidental nested deletion.'
    ]
  },
  {
    name: 'run',
    signature: 'golunch run <alias> [flags] [-- <args>]',
    summary: 'Headless prompt execution with normalized streaming events or CLI passthrough',
    description: 'Executes the agent in headless prompt mode with structured events, or passes arguments directly through to the wrapped agent binary inside the isolated environment. Acquires a shared flock (or exclusive flock with --continue without --session).',
    flags: [
      { flag: '--prompt', type: 'string', description: 'Prompt text for headless non-interactive agent execution' },
      { flag: '--stdin', type: 'boolean', description: 'Read prompt content from standard input' },
      { flag: '--jsonl', type: 'boolean', description: 'Emit normalized streaming NDJSON events to stdout' },
      { flag: '--dry-run', type: 'boolean', description: 'Print resolved argv, environment variables, and proxy; do not start process' },
      { flag: '--model', type: 'string', description: 'Target model alias (e.g. anthropic/claude-sonnet-4-5)' },
      { flag: '--provider', type: 'string', description: 'Override AI provider gateway' },
      { flag: '--timeout', type: 'duration', description: 'Hard execution timeout (e.g. 90s, 5m, 1h)' },
      { flag: '--thinking', type: 'boolean', description: 'Stream internal reasoning/thinking events' },
      { flag: '--auto-approve', type: 'boolean', description: 'Auto-approve agent tool invocations' },
      { flag: '--proxy', type: 'string', description: 'Override proxy for this run only (highest precedence)' },
      { flag: '--quiet', type: 'boolean', description: 'Suppress progress output and stderr noise' }
    ],
    examples: [
      {
        title: 'Headless prompt with streaming NDJSON',
        command: 'golunch run work --prompt "what tests are failing?" -m anthropic/claude-sonnet-4-5 --timeout 5m --jsonl',
        output: '{"type":"tool_call","agent":"cline","tool":{"name":"list_files","input":{}}}\n{"type":"text","agent":"cline","text":"Two suites fail ..."}\n{"type":"usage","agent":"cline","usage":{"input_tokens":41203,"output_tokens":812,"cost":0.19}}'
      },
      {
        title: 'Direct command passthrough',
        command: 'golunch run work -- make test',
        explanation: 'Runs make test using the work instance environment. Child exit code is preserved verbatim.'
      },
      {
        title: 'Dry run inspection',
        command: 'golunch run work --prompt "hello" --dry-run',
        explanation: 'Prints the resolved argv, injected environment variables, and proxy layer without acquiring locks or starting processes.'
      }
    ]
  },
  {
    name: 'seed',
    signature: 'golunch seed <alias> [group...] [--all]',
    summary: 'Synchronize host MCP servers, skills, and plugins without leaking credentials',
    description: 'Copies configuration files, tool definitions, and skill packages from your host setup into the isolated instance. Explicitly inspects and blocks credentials (such as providers.json or files containing apiKey) and alerts on absolute host paths.',
    flags: [
      { flag: '--all', type: 'boolean', description: 'Seed all registered groups (mcp, skills, config, commands)' },
      { flag: '--with-deps', type: 'boolean', description: 'Also copy node_modules (can be large, omitted by default)' },
      { flag: '--dry-run', type: 'boolean', description: 'Preview files that would be copied or refused without writing to disk' },
      { flag: '--force', type: 'boolean', description: 'Overwrite existing files in target instance directory' },
      { flag: '--host-home', type: 'string', description: 'Read source configurations from a custom directory instead of host $HOME' }
    ],
    examples: [
      {
        title: 'Seed all MCP servers and skills',
        command: 'golunch seed work --all',
        output: 'absent  opencode.jsonc, agents\ncopy    ~/.config/opencode/opencode.json -> config/opencode/opencode.json\ntree    ~/.config/opencode/skill -> config/opencode/skill (7 files)\ntree    ~/.config/opencode/plugin -> config/opencode/plugin (5 files)\nseeded work (35 files copied)\nwarn: config/opencode/opencode.json holds apiKey; written 0600'
      },
      {
        title: 'Dry run MCP seed only',
        command: 'golunch seed work mcp --dry-run',
        explanation: 'Lists what MCP servers would be copied into work without modifying any instance files.'
      }
    ],
    notes: [
      'Refuses sensitive credentials by filename (e.g. cline providers.json containing plaintext keys).',
      'If a copied file turns out to contain an API key, chmod 0600 is applied and an explicit warning is printed.'
    ]
  },
  {
    name: 'doctor',
    signature: 'golunch doctor [alias] [--json]',
    summary: 'Verify instance isolation, binary integrity, flock status, and drift',
    description: 'Runs diagnostic checks across all instances or a target instance: confirms login isolation (verifies instance is unauthenticated), checks binary existence, verifies launcher hash consistency, checks advisory flock state, and inspects install script audit drift.',
    flags: [
      { flag: '--json', type: 'boolean', description: 'Output diagnostic report as structured JSON' }
    ],
    examples: [
      {
        title: 'Run full diagnostics on an instance',
        command: 'golunch doctor work',
        output: '✓ Isolation: clean (instance is logged out)\n✓ Binary: /home/user/.golunch/instances/work/bin/cline (executable, non-root)\n✓ Launcher: in sync with BuildEnv\n✓ Lock: flock available (not busy)\n✓ Drift: audit script matches recorded sha256'
      }
    ]
  },
  {
    name: 'task',
    signature: 'golunch task <taskfile.json> [flags]',
    summary: 'Fan N prompts out across instances with per-task timeouts and summaries',
    description: 'Executes parallel batches of prompts across multiple instances. Supports task matrix expansion, individual timeout ceilings, concurrency throttles (-parallel), and NDJSON event streaming with RAM ring buffers.',
    flags: [
      { flag: '--parallel', type: 'int', description: 'Maximum concurrent worker tasks', default: '3' },
      { flag: '--out', type: 'string', description: 'Write structured JSON execution summary to file' },
      { flag: '--quiet', type: 'boolean', description: 'Suppress per-event terminal streaming' },
      { flag: '--ring', type: 'int', description: 'Number of recent events kept in RAM buffer per task' },
      { flag: '--dry-run', type: 'boolean', description: 'Validate taskfile schema and instance availability without running' }
    ],
    examples: [
      {
        title: 'Execute multi-instance sweep',
        command: 'golunch task sweep.json --parallel 4 --out summary.json',
        explanation: 'Fans out tasks defined in sweep.json across target instances. One failing task does not halt remaining tasks.'
      }
    ]
  },
  {
    name: 'clone',
    signature: 'golunch clone <src> <dst> [flags]',
    summary: 'Create a second identity: copies the binary and config, never the login',
    description: 'Duplicates an instance structure, binary, and custom skills into a new alias. By default skips credentials and session databases (e.g. kilo.db is ~1.2GB) so the clone begins with a fresh login identity.',
    flags: [
      { flag: '--copy-data', type: 'boolean', description: 'Copy conversation state/data directory (warns about credential leakage)' },
      { flag: '--proxy', type: 'string', description: 'Assign distinct proxy configuration to the new clone' },
      { flag: '--link', type: 'boolean', description: 'Create command link in ~/.local/bin/<dst>', default: 'true' },
      { flag: '--yes', type: 'boolean', description: 'Bypass interactive confirmation prompts' }
    ],
    examples: [
      {
        title: 'Clone work instance for personal project',
        command: 'golunch clone work personal --proxy none',
        explanation: 'Copies the agent binary and MCP setup from work to personal, but leaves login credentials uncopied and clears proxy.'
      }
    ]
  },
  {
    name: 'install',
    signature: 'golunch install <alias> [--url <url> | --script <path>] [flags]',
    summary: 'Download agent binary into the instance private bin/ directory',
    description: 'Fetches agent installer via the resolved proxy profile, computes and validates SHA256 checksum, executes inside the instance with redirected HOME (refusing uid 0), and links the resulting binary to <instance>/bin/.',
    flags: [
      { flag: '--url', type: 'string', description: 'HTTPS URL of installer script or package' },
      { flag: '--script', type: 'string', description: 'Local path to installer script to execute' },
      { flag: '--sha256', type: 'string', description: 'Expected sha256 checksum of downloaded script' },
      { flag: '--timeout', type: 'duration', description: 'Download and execution timeout', default: '5m' },
      { flag: '--dry-run', type: 'boolean', description: 'Fetch and print sha256 without executing' }
    ],
    examples: [
      {
        title: 'Securely install agent from verified URL',
        command: 'golunch install work --url https://agent.dev/install.sh --sha256 8a4b3c...',
        explanation: 'Script is saved to logs/ for audit, sha256 verified, executed under non-root user, and placed in bin/.'
      }
    ]
  },
  {
    name: 'ls',
    signature: 'golunch ls [-l | --json]',
    summary: 'List all configured instances with metadata and health status',
    description: 'Enumerates instances in ~/.golunch/instances/. Default mode is instantaneous (reads metadata.toml without disk walk). Long listing (-l) reports disk size, binary version, proxy source, and last run timestamp.',
    flags: [
      { flag: '-l', type: 'boolean', description: 'Long listing including tree disk size, binary version, proxy provenance' },
      { flag: '--json', type: 'boolean', description: 'Output instance listing in structured JSON format' }
    ],
    examples: [
      {
        title: 'List instances with details',
        command: 'golunch ls -l',
        output: 'ALIAS     AGENT    PROXY            SIZE    LAST RUN             STATUS\nwork      cline    http://127.0...  142 MB  2026-10-01 13:45    idle\nresearch  kilo     corp             1.3 GB  2026-10-01 12:10    idle\nstaging   opencode none             84 MB   2026-09-30 18:22    idle'
      }
    ]
  },
  {
    name: 'shell',
    signature: 'golunch shell <alias> [-c "command"] [flags]',
    summary: 'Launch an interactive login shell inside the instance environment',
    description: 'Spawns your preferred shell with all isolated environment variables (HOME, XDG base directories, TMPDIR, 8 proxy variables, and instance bin/ prepended to PATH).',
    flags: [
      { flag: '-c', type: 'string', description: 'Execute command string in instance shell and exit' },
      { flag: '--proxy', type: 'string', description: 'Override proxy profile for this shell session' },
      { flag: '--noproxy', type: 'boolean', description: 'Unset all proxy variables for this shell session' }
    ],
    examples: [
      {
        title: 'Open interactive shell',
        command: 'golunch shell work',
        explanation: 'Launches bash with work instance HOME and PATH. Running `which cline` returns work/bin/cline.'
      }
    ]
  },
  {
    name: 'env',
    signature: 'golunch env <alias> [--all | --shell]',
    summary: 'Inspect the fully resolved child environment and proxy provenance',
    description: 'Displays the precise environment variables that will be passed to child processes, along with audit annotations explaining which configuration layer set each proxy variable.',
    flags: [
      { flag: '--all', type: 'boolean', description: 'Display all host and injected environment variables' },
      { flag: '--shell', type: 'boolean', description: 'Output as export KEY=VALUE format suitable for sourcing' }
    ],
    examples: [
      {
        title: 'Inspect proxy provenance',
        command: 'golunch env work',
        output: 'HOME=/home/user/.golunch/instances/work/home\nXDG_CONFIG_HOME=/home/user/.golunch/instances/work/config\nHTTP_PROXY=http://127.0.0.1:7890 (source: work/metadata.toml)\nNO_PROXY=localhost,127.0.0.1,::1,.localhost (source: loopback floor)'
      }
    ]
  },
  {
    name: 'refresh',
    signature: 'golunch refresh [alias]',
    summary: 'Regenerate launcher wrapper scripts from current configuration',
    description: 'Rebuilds ~/.golunch/instances/<alias>/launcher scripts to ensure zero drift against BuildEnv without disturbing instance data.',
    flags: [],
    examples: [
      {
        title: 'Refresh all instance launchers',
        command: 'golunch refresh',
        explanation: 'Synchronizes all launchers with the latest golunch build environment definition.'
      }
    ]
  },
  {
    name: 'rm',
    signature: 'golunch rm <alias> [--yes] [--keep-link]',
    summary: 'Delete an instance directory and remove its command link',
    description: 'Acquires an exclusive flock and removes the instance directory tree from ~/.golunch/instances/<alias> and removes ~/.local/bin/<alias>. Refuses to run if the instance is currently busy.',
    flags: [
      { flag: '--yes', type: 'boolean', description: 'Skip interactive confirmation prompt' },
      { flag: '--keep-link', type: 'boolean', description: 'Do not remove ~/.local/bin/<alias> symlink' }
    ],
    examples: [
      {
        title: 'Remove temporary instance',
        command: 'golunch rm scratch --yes',
        explanation: 'Safely releases any flock and purges the instance tree.'
      }
    ]
  },
  {
    name: 'config',
    signature: 'golunch config [--init | --path]',
    summary: 'Display or initialize global configuration and named proxy profiles',
    description: 'Manages ~/.golunch/config.toml, where global defaults (default shell, timeouts) and named corporate/personal proxy profiles are registered.',
    flags: [
      { flag: '--init', type: 'boolean', description: 'Write well-commented sample config.toml if none exists' },
      { flag: '--path', type: 'boolean', description: 'Print absolute filesystem path to config.toml' }
    ],
    examples: [
      {
        title: 'Initialize config file',
        command: 'golunch config --init',
        explanation: 'Creates ~/.golunch/config.toml with commented template for proxy profiles and default shells.'
      }
    ]
  },
  {
    name: 'version',
    signature: 'golunch version',
    summary: 'Print the module version, Go runtime, and build metadata',
    description: 'Outputs the version string embedded at build time (either Go module version or git describe via -ldflags).',
    flags: [],
    examples: [
      {
        title: 'Check version',
        command: 'golunch version',
        output: 'golunch v1.0.0 (go1.27.1 linux/amd64)'
      }
    ]
  }
];
