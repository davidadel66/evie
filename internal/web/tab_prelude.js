(() => {
  // Evie's partial WebRTC block (workspace-tabs.decisions.md, 2026-10-12):
  // WebRTC can signal out where no page policy reaches, so it is removed
  // before the tab's own code runs, and frames cannot hand back a fresh
  // window that still has it. Defence in depth, not a seal.
  const gone = { value: undefined, writable: false, configurable: false };
  for (const name of ['RTCPeerConnection', 'webkitRTCPeerConnection', 'RTCDataChannel', 'RTCSessionDescription', 'RTCIceCandidate',
    'RTCRtpSender', 'RTCRtpReceiver', 'RTCRtpTransceiver', 'RTCDtlsTransport', 'RTCIceTransport', 'RTCSctpTransport', 'RTCCertificate']) {
    try { Object.defineProperty(window, name, gone); } catch (_) { /* already fixed */ }
  }
  for (const element of [window.HTMLIFrameElement, window.HTMLFrameElement, window.HTMLObjectElement]) {
    if (!element) continue;
    for (const property of ['contentWindow', 'contentDocument']) {
      try { Object.defineProperty(element.prototype, property, { get() { return null; }, configurable: false }); } catch (_) { /* already fixed */ }
    }
  }
})();
