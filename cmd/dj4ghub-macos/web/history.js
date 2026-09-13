(() => {
  const dialog = document.createElement('dialog');
  dialog.style.cssText = 'width:min(760px,90vw);max-height:80vh;border:1px solid #ddd;border-radius:16px;padding:24px;color:inherit;background:var(--surface,#fff)';
  dialog.innerHTML = '<h2>通信记录</h2><p>记录保存在本机。无法确认来源的旧短信归入“未归属”；时长为观测值。</p><select aria-label="筛选 SIM 卡"></select> <button type="button" data-refresh>刷新</button> <button type="button" data-close>关闭</button><p role="status"></p><div data-rows></div><button type="button" data-more>加载更多</button>';
  document.body.append(dialog);
  const select = dialog.querySelector('select');
  const rows = dialog.querySelector('[data-rows]');
  const status = dialog.querySelector('[role=status]');
  const more = dialog.querySelector('[data-more]');
  let kind = 'sms', offset = 0, loading = false;
  const labels = { received:'已接收', sent:'已提交模块', failed_or_partial:'失败或部分发送', failed:'拨号失败', observed:'呼叫中', connected:'通话中', ended:'已结束', unanswered:'未接通', missed:'未接来电', interrupted:'记录中断' };
  function options(cards, value) {
    select.replaceChildren();
    for (const [key, label] of [['current','当前卡片'],['all','全部卡片'],['unknown','未归属'],...Object.entries(cards)]) {
      const option = document.createElement('option'); option.value = key; option.textContent = label; select.append(option);
    }
    select.value = value;
  }
  async function load(append = false) {
    if (loading) return;
    loading = true; select.disabled = true; more.disabled = true;
    if (!append) { offset = 0; rows.replaceChildren(); }
    const card = select.value || 'current';
    try {
      const response = await fetch(`/api/history?kind=${kind}&card=${encodeURIComponent(card)}&offset=${offset}`);
      if (!response.ok) throw new Error('无法读取记录，请确认服务已更新');
      const data = await response.json();
      options(data.cards, card);
      for (const record of data.records) {
        const article = document.createElement('article'); article.style.cssText = 'padding:16px 0;border-bottom:1px solid #ddd';
        const title = document.createElement('strong'); title.textContent = `${record.number} · ${record.direction === 'incoming' ? '接收 / 来电' : '发送 / 去电'} · ${labels[record.state] || record.state}`;
        const meta = document.createElement('p'); meta.textContent = `${record.iccid ? 'SIM · ' + record.iccid.slice(-4) : '未归属'} · ${new Date(record.started).toLocaleString()}`;
        const body = document.createElement('p'); body.style.whiteSpace = 'pre-wrap'; body.textContent = kind === 'sms' ? record.content : record.state === 'ended' ? `观测通话时长：${record.duration_seconds} 秒` : '';
        article.append(title, meta, body); rows.append(article);
      }
      offset = data.next_offset; status.textContent = `共 ${data.total} 条`; more.hidden = offset >= data.total;
    } catch (error) { status.textContent = error.message; more.hidden = true; }
    finally { loading = false; select.disabled = false; more.disabled = false; }
  }
  document.querySelectorAll('[data-history]').forEach(button => button.addEventListener('click', () => {
    if (loading) return;
    kind = button.dataset.history; options({}, 'current'); dialog.showModal(); load();
  }));
  select.addEventListener('change', () => load());
  dialog.querySelector('[data-refresh]').onclick = () => load();
  dialog.querySelector('[data-close]').onclick = () => dialog.close();
  more.onclick = () => load(true);
  setInterval(() => { if (dialog.open && select.value === 'current' && offset <= 100) load(); }, 10000);
})();
