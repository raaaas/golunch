export type AgentType = 'cline' | 'kilo' | 'opencode';

export interface AgentInfo {
  id: AgentType;
  name: string;
  tagline: string;
  description: string;
  mcpPath: string;
  skillsPath: string;
  credentialsFile: string;
  eventSchema: 'wrapped_agent_event' | 'flat_ndjson';
  quirks: string[];
}

export interface CliCommand {
  name: string;
  signature: string;
  summary: string;
  description: string;
  flags: {
    flag: string;
    type: string;
    description: string;
    default?: string;
  }[];
  examples: {
    title: string;
    command: string;
    explanation?: string;
    output?: string;
  }[];
  notes?: string[];
}

export interface DocSection {
  id: string;
  title: string;
  category: string;
  readTime: string;
  summary: string;
  content: DocBlock[];
}

export type DocBlock =
  | { type: 'heading'; level: 2 | 3; text: string }
  | { type: 'paragraph'; text: string }
  | { type: 'code'; language: string; code: string; title?: string }
  | { type: 'table'; headers: string[]; rows: string[][] }
  | { type: 'callout'; variant: 'info' | 'warning' | 'tip'; title: string; text: string }
  | { type: 'list'; items: string[] };

export interface NormalizedEvent {
  type: 'thinking' | 'tool_call' | 'text' | 'usage' | 'status' | 'error';
  agent: string;
  timestamp?: string;
  text?: string;
  tool?: {
    name: string;
    input: Record<string, unknown>;
  };
  usage?: {
    input_tokens: number;
    output_tokens: number;
    cost: number;
  };
}

export interface InstancePreview {
  alias: string;
  agent: AgentType;
  proxy: string;
  status: 'idle' | 'running' | 'locked';
  lastRun?: string;
  path: string;
  mcpCount: number;
  skillsCount: number;
}
