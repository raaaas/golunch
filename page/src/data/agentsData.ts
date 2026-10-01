import { AgentInfo } from '../types.ts';

export const SUPPORTED_AGENTS: AgentInfo[] = [
  {
    id: 'cline',
    name: 'Cline',
    tagline: 'Autonomous coding agent with deep MCP integration',
    description: 'Autonomous coding agent supporting tools and MCP server integrations. Requires specialized auto-approval handling and strict credential safety.',
    mcpPath: '~/.cline/data/settings/cline_mcp_settings.json',
    skillsPath: '~/.cline/skills/',
    credentialsFile: '~/.cline/data/settings/providers.json',
    eventSchema: 'wrapped_agent_event',
    quirks: [
      'Cline has no -y flag. Auto-approval is specified via --auto-approve <boolean>.',
      'Keeps providers.json next to MCP settings holding apiKey and accessToken in plaintext. golunch treats it as a credential, refuses it in seed, and checks it in doctor.',
      'Streams NDJSON with an agent_event wrapper format.'
    ]
  },
  {
    id: 'kilo',
    name: 'Kilo',
    tagline: 'High-throughput developer agent (fork of OpenCode)',
    description: 'High-performance code orchestration agent. Utilizes a local SQLite database for conversation states and flat NDJSON event streaming.',
    mcpPath: 'kilo.jsonc (key: "mcp")',
    skillsPath: 'agents/, node_modules/',
    credentialsFile: 'XDG_DATA_HOME/kilo.db',
    eventSchema: 'flat_ndjson',
    quirks: [
      'Emits flat records ({type, sessionID, part}), not the agent_event wrapper. golunch normalizes both seamlessly.',
      'Stores credentials in XDG_DATA_HOME; kilo.db can reach ~1.2 GB, so golunch ls avoids tree walking unless given -l.',
      'Overriding XDG_CONFIG_HOME means fresh instances lack plugins/npm modules until golunch seed --with-deps is called.'
    ]
  },
  {
    id: 'opencode',
    name: 'OpenCode',
    tagline: 'Modular open-source terminal coding assistant',
    description: 'Lightweight, plugin-rich terminal agent with support for community skills, custom tools, and multi-provider models.',
    mcpPath: 'opencode.json (key: "mcp")',
    skillsPath: 'skill/, plugin/, agent/, command/',
    credentialsFile: 'XDG_DATA_HOME/auth.json',
    eventSchema: 'flat_ndjson',
    quirks: [
      'Shares agent driver factory with kilo with discrete configuration directories.',
      'Absolute paths in opencode.json reference host scripts; golunch warns on seed so local bin scripts are used instead.',
      'Supports granular seeding of skill, plugin, and command directories.'
    ]
  }
];
