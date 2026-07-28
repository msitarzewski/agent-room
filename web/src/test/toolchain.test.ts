import packageManifest from "../../package.json";

describe("frontend runtime boundary", () => {
  it("pins the CI toolchain and keeps the development server on loopback", () => {
    expect(packageManifest.packageManager).toBe("npm@11.6.0");
    expect(packageManifest.engines).toEqual({ node: "24.8.0", npm: "11.6.0" });
    expect(packageManifest.scripts.dev).toBe("vite --host 127.0.0.1");
    expect(packageManifest.scripts.dev).not.toContain("0.0.0.0");
    expect(packageManifest.scripts["api:lint"]).toContain("--extends recommended-strict");
    expect(packageManifest.scripts["test:e2e:all"]).toBe(
      "playwright test --project=chromium --project=firefox --project=webkit",
    );
  });
});
