export interface Project {
  service_type: "http" | "worker";
  id: string;
  name: string;
  source_type: "image" | "git";
  source: string;
  branch: string;
  build_method: string;
  dockerfile: string;
  build_context: string;
  port: number;
  domain: string;
  health_path: string;
  auto_deploy: boolean;
  memory_mb: number;
  cpu: number;
  status: string;
  active_image: string;
  active_container: string;
  created_at: string;
}
export interface Deployment {
  id: string;
  project_id: string;
  project_name: string;
  status: string;
  image: string;
  commit_sha: string;
  error: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}
export interface Session {
  needs_setup: boolean;
  totp_enabled: boolean;
  email: string;
  csrf: string;
  provider: string;
}
export interface System {
  projects: number;
  running: number;
  failed: number;
  queued: number;
  provider: string;
}
