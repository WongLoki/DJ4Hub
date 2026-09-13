const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname, '../cmd/dj4ghub-macos/web/phone.js'), 'utf8');

function harness() {
  const elements = new Map();
  const requests = [];
  const context = {
    moduleAudioBusy: false, callPollBusy: false, phoneActionBusy: false,
    previousCallsPresent: true, callStarted: new Map(), moduleAudioToken: 'session',
    document: {querySelectorAll: () => []},
    $: id => { if (!elements.has(id)) elements.set(id, {checked: true}); return elements.get(id); },
    stopPhoneAudio: () => requests.push('stop-media'),
    ensureModuleAudio: async () => requests.push('ensure'),
    connectPhoneAudio: async () => requests.push('connect'),
    api: async (path, options) => { requests.push(options ? JSON.parse(options.body).action : path); return {calls: []}; }
  };
  vm.createContext(context);
  vm.runInContext(source.slice(source.indexOf('async function refreshCalls()'), source.indexOf("$('#phone-dial').onclick")), context);
  return {context, requests, elements};
}

test('remote hangup closes media but preserves prepared module', async () => {
  const {context, requests} = harness();
  await context.refreshCalls();
  assert.deepEqual(requests, ['/api/calls', 'stop-media']);
  assert.equal(context.moduleAudioToken, 'session');
});
test('explicit hangup does not stop module session', async () => {
  const {context, requests} = harness();
  await context.phoneAction('hangup');
  assert.equal(requests[0], 'hangup');
  assert.ok(!requests.includes('ensure'));
  assert.equal(context.moduleAudioToken, 'session');
});
test('dial initializes and connects audio before sending dial command', async () => {
  const {context, requests} = harness();
  await context.phoneAction('dial', {number: '+6421000000'});
  assert.deepEqual(requests.slice(0, 3), ['ensure', 'connect', 'dial']);
});
test('failed audio initialization never dials', async () => {
  const {context, requests} = harness();
  context.ensureModuleAudio = async () => { throw new Error('not ready'); };
  await context.phoneAction('dial', {number: '+6421000000'});
  assert.ok(!requests.includes('dial'));
  assert.ok(requests.includes('stop-media'));
});
test('explicit control-only mode works without audio dependencies', async () => {
  const {context, requests} = harness();
  context.$('#phone-use-audio').checked = false;
  await context.phoneAction('answer');
  assert.equal(requests[0], 'answer');
  assert.ok(!requests.includes('ensure'));
});
test('USB preparation suppresses misleading call poll failure', async () => {
  const {context, requests, elements} = harness();
  context.moduleAudioBusy = true;
  await context.refreshCalls();
  assert.equal(requests.length, 0);
  assert.match(elements.get('#phone-status').textContent, /USB/);
});
