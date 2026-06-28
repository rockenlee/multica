import { describe, expect, it } from "vitest";
import {
  CLOUD_RELEASE_REPO,
  SELF_HOST_RELEASE_REPO,
  originFromHeaders,
  resolveDownloadSource,
} from "./download-source";

function headers(values: Record<string, string>): Headers {
  return new Headers(values);
}

describe("resolveDownloadSource", () => {
  it("keeps managed cloud on the official release source", () => {
    const source = resolveDownloadSource({
      origin: "https://multica.ai",
      env: {},
    });

    expect(source.releaseRepo).toBe(CLOUD_RELEASE_REPO);
    expect(source.allReleasesUrl).toBe("https://github.com/multica-ai/multica/releases");
    expect(source.installCommand).toBe(
      "curl -fsSL https://raw.githubusercontent.com/multica-ai/multica/main/scripts/install.sh | bash",
    );
    expect(source.setupCommand).toBe("multica setup");
  });

  it("uses the self-host release source and current origin off managed cloud", () => {
    const source = resolveDownloadSource({
      origin: "http://172.16.40.99:13000",
      env: {},
    });

    expect(source.releaseRepo).toBe(SELF_HOST_RELEASE_REPO);
    expect(source.allReleasesUrl).toBe("https://github.com/rockenlee/multica/releases");
    expect(source.installCommand).toBe(
      "curl -fsSL https://raw.githubusercontent.com/rockenlee/multica/main/scripts/install.sh | bash",
    );
    expect(source.setupCommand).toBe(
      "multica setup self-host --server-url http://172.16.40.99:13000 --app-url http://172.16.40.99:13000",
    );
  });

  it("accepts an explicit GitHub repo URL override", () => {
    const source = resolveDownloadSource({
      origin: "http://example.internal",
      env: {
        MULTICA_RELEASE_REPO: "https://github.com/acme/multica.git",
        MULTICA_INSTALL_REF: "release",
      },
    });

    expect(source.releaseRepo).toBe("acme/multica");
    expect(source.installCommand).toBe(
      "curl -fsSL https://raw.githubusercontent.com/acme/multica/release/scripts/install.sh | bash",
    );
  });
});

describe("originFromHeaders", () => {
  it("derives http for a private self-host address without forwarded proto", () => {
    expect(originFromHeaders(headers({ host: "172.16.40.99:13000" }))).toBe(
      "http://172.16.40.99:13000",
    );
  });

  it("uses forwarded host and proto when a proxy provides them", () => {
    expect(
      originFromHeaders(
        headers({
          host: "internal:3000",
          "x-forwarded-host": "multica.example.com",
          "x-forwarded-proto": "https",
        }),
      ),
    ).toBe("https://multica.example.com");
  });
});
