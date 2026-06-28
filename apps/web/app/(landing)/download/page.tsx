import type { Metadata } from "next";
import { headers } from "next/headers";
import { fetchLatestRelease } from "@/features/landing/utils/github-release";
import {
  originFromHeaders,
  resolveDownloadSource,
} from "@/features/landing/utils/download-source";
import { DownloadClient } from "./download-client";

// Vercel ISR: the server fetch inside fetchLatestRelease carries
// `next: { revalidate: 300 }`, which makes GitHub API cost at most
// one request per region per 5 minutes. Page-level revalidate mirrors
// that window so the first paint also refreshes every 5 minutes.
export const revalidate = 300;

export const metadata: Metadata = {
  title: "Download Multica",
  description:
    "Download Multica for macOS, Windows, or Linux — or install the CLI for servers and remote dev boxes.",
  openGraph: {
    title: "Download Multica",
    description:
      "Get the Multica desktop app with a bundled daemon, or install the CLI for servers and remote dev boxes.",
    url: "/download",
  },
  alternates: {
    canonical: "/download",
  },
};

export default async function DownloadPage() {
  const headerList = await headers();
  const downloadSource = resolveDownloadSource({
    origin: originFromHeaders(headerList),
  });
  const release = await fetchLatestRelease(downloadSource.releaseRepo);
  return <DownloadClient release={release} downloadSource={downloadSource} />;
}
