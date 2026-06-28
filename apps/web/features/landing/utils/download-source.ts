export const CLOUD_RELEASE_REPO = "multica-ai/multica";
export const SELF_HOST_RELEASE_REPO = "rockenlee/multica";

const CLOUD_HOSTS = ["multica.ai", "www.multica.ai"];
const DEFAULT_INSTALL_REF = "main";

type HeaderReader = Pick<Headers, "get">;
type Env = Record<string, string | undefined>;

export interface DownloadSource {
  releaseRepo: string;
  releasesApiUrl: string;
  allReleasesUrl: string;
  installCommand: string;
  setupCommand: string;
}

export function resolveDownloadSource({
  origin,
  env = process.env,
}: {
  origin: string | null;
  env?: Env;
}): DownloadSource {
  const normalizedOrigin = normalizeOrigin(origin);
  const configuredRepo = normalizeGithubRepo(
    env.MULTICA_RELEASE_REPO ?? env.MULTICA_GITHUB_REPO,
  );
  const releaseRepo =
    configuredRepo ??
    (isManagedCloudOrigin(normalizedOrigin)
      ? CLOUD_RELEASE_REPO
      : SELF_HOST_RELEASE_REPO);
  const installRef = env.MULTICA_INSTALL_REF?.trim() || DEFAULT_INSTALL_REF;

  return {
    releaseRepo,
    releasesApiUrl: githubReleasesApiUrl(releaseRepo),
    allReleasesUrl: githubReleasesUrl(releaseRepo),
    installCommand: `curl -fsSL https://raw.githubusercontent.com/${releaseRepo}/${installRef}/scripts/install.sh | bash`,
    setupCommand:
      normalizedOrigin && !isManagedCloudOrigin(normalizedOrigin)
        ? `multica setup self-host --server-url ${normalizedOrigin} --app-url ${normalizedOrigin}`
        : "multica setup",
  };
}

export function originFromHeaders(headers: HeaderReader): string | null {
  const rawHost = firstHeaderValue(
    headers.get("x-forwarded-host") ?? headers.get("host"),
  );
  if (!rawHost) return null;
  const host = rawHost.trim();
  if (!host) return null;
  const proto =
    firstHeaderValue(headers.get("x-forwarded-proto")) ??
    defaultProtocolForHost(host);
  return normalizeOrigin(`${proto}://${host}`);
}

export function githubReleasesApiUrl(repo: string): string {
  return `https://api.github.com/repos/${repo}/releases?per_page=2`;
}

export function githubReleasesUrl(repo: string): string {
  return `https://github.com/${repo}/releases`;
}

function normalizeGithubRepo(value: string | undefined): string | null {
  const trimmed = value?.trim();
  if (!trimmed) return null;
  const withoutGithubPrefix = trimmed
    .replace(/^https:\/\/github\.com\//, "")
    .replace(/^git@github\.com:/, "")
    .replace(/\.git$/, "")
    .replace(/^\/+|\/+$/g, "");
  return /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(withoutGithubPrefix)
    ? withoutGithubPrefix
    : null;
}

function normalizeOrigin(value: string | null | undefined): string | null {
  const trimmed = value?.trim();
  if (!trimmed) return null;
  try {
    const url = new URL(trimmed);
    return url.origin.replace(/\/+$/, "");
  } catch {
    return null;
  }
}

function isManagedCloudOrigin(origin: string | null): boolean {
  if (!origin) return false;
  try {
    const host = new URL(origin).hostname;
    return CLOUD_HOSTS.includes(host) || host.endsWith(".multica.ai");
  } catch {
    return false;
  }
}

function firstHeaderValue(value: string | null | undefined): string | null {
  return value?.split(",")[0]?.trim() || null;
}

function defaultProtocolForHost(hostWithOptionalPort: string): "http" | "https" {
  const host =
    hostWithOptionalPort.replace(/^\[/, "").replace(/\]$/, "").split(":")[0] ??
    "";
  if (
    host === "localhost" ||
    host === "127.0.0.1" ||
    host === "::1" ||
    host.startsWith("10.") ||
    host.startsWith("192.168.") ||
    isPrivate172Host(host)
  ) {
    return "http";
  }
  return "https";
}

function isPrivate172Host(host: string): boolean {
  const match = /^172\.(\d+)\./.exec(host);
  if (!match) return false;
  const secondOctet = Number(match[1]);
  return secondOctet >= 16 && secondOctet <= 31;
}
