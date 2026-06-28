import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { configStore } from "@multica/core/config";
import enCommon from "../../locales/en/common.json";
import enOnboarding from "../../locales/en/onboarding.json";
import { CliInstallInstructions } from "./cli-install-instructions";

const TEST_RESOURCES = { en: { common: enCommon, onboarding: enOnboarding } };

const ligatureClasses = [
  "[font-variant-ligatures:none]",
  "[font-feature-settings:'liga'_0]",
];

function resetConfigStore() {
  configStore.setState({
    cdnDomain: "",
    cdnSigned: false,
    allowSignup: true,
    googleClientId: "",
    daemonServerUrl: "",
    daemonAppUrl: "",
    workspaceCreationDisabled: false,
  });
}

function renderInstructions(config?: {
  daemonServerUrl?: string;
  daemonAppUrl?: string;
}) {
  resetConfigStore();
  if (config) {
    configStore.getState().setDaemonConfig(config);
  }
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <CliInstallInstructions />
    </I18nProvider>,
  );
}

describe("CliInstallInstructions", () => {
  beforeEach(() => {
    resetConfigStore();
  });

  it("disables font ligatures in CLI command code", () => {
    renderInstructions({
      daemonServerUrl: "https://api.example.com",
      daemonAppUrl: "https://app.example.com",
    });

    expect(
      screen.getByText(
        "multica setup self-host --server-url https://api.example.com --app-url https://app.example.com",
      ),
    ).toHaveClass(...ligatureClasses);
  });

  it("uses configured self-host URLs when available", () => {
    renderInstructions({
      daemonServerUrl: "http://172.16.40.99:13000",
      daemonAppUrl: "http://172.16.40.99:13000",
    });

    expect(screen.getByText(/raw\.githubusercontent\.com\/rockenlee\/multica/)).toBeTruthy();
    expect(
      screen.getByText(
        "multica setup self-host --server-url http://172.16.40.99:13000 --app-url http://172.16.40.99:13000",
      ),
    ).toBeTruthy();
  });
});
