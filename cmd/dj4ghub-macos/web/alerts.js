(() => {
  let audio, enabled = false, initialized = false, busy = false;
  const sms = new Set(), calls = new Set();
  const buttons = document.querySelectorAll('[data-enable-alerts]');
  buttons.forEach(button => button.addEventListener('click', async () => {
    try {
      if (!audio) audio = new AudioContext();
      await audio.resume(); enabled = !enabled;
      buttons.forEach(b => b.textContent = enabled ? '关闭来电／短信提示音' : '开启来电／短信提示音');
    } catch { button.textContent = '浏览器未允许声音，请重试'; }
  }));
  function beep(call) {
    if (!enabled || !audio || audio.state !== 'running') return;
    const oscillator = audio.createOscillator(), gain = audio.createGain();
    oscillator.frequency.value = call ? 660 : 880;
    gain.gain.setValueAtTime(0.08, audio.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, audio.currentTime + 0.5);
    oscillator.connect(gain); gain.connect(audio.destination);
    oscillator.start(); oscillator.stop(audio.currentTime + 0.5);
    oscillator.onended = () => { oscillator.disconnect(); gain.disconnect(); };
  }
  async function poll() {
    if (busy) return; busy = true;
    try {
      const response = await fetch('/api/alerts'); if (!response.ok) return;
      const data = await response.json();
      const newSMS = initialized && data.sms.some(id => !sms.has(id));
      const newCall = data.ringing.some(id => !calls.has(id));
      data.sms.forEach(id => sms.add(id)); data.ringing.forEach(id => calls.add(id));
      initialized = true;
      if (newCall || newSMS) beep(newCall);
    } catch { /* Preserve event identities across transient disconnections. */ }
    finally { busy = false; }
  }
  void poll(); setInterval(poll, 3000);
})();
