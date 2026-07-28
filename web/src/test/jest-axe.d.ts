declare module "jest-axe" {
  interface AxeResult {
    violations: Array<{
      id: string;
      impact: string | null;
      description: string;
      nodes: unknown[];
    }>;
  }

  export function axe(
    html: Element | string,
    options?: Record<string, unknown>,
  ): Promise<AxeResult>;
}
