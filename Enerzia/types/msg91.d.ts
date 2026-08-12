/** MSG91 SendOTP widget — hand-written minimal type declarations for strict TypeScript. */

interface MSG91WidgetConfig {
  widgetId: string;
  tokenAuth: string;
  exposeMethods?: boolean;
  /**
   * Called when the shopper completes the OTP flow. The payload shape is
   * undocumented by MSG91 — extract the token defensively at the call site
   * rather than asserting a specific key.
   */
  success: (data: unknown) => void;
  failure: (error: unknown) => void;
}

interface Window {
  /**
   * Optional — initSendOTP may be blocked by an ad blocker or fail to load.
   * Always check `typeof window.initSendOTP === 'function'` before calling.
   */
  initSendOTP?: (config: MSG91WidgetConfig) => void;
}
