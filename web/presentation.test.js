// Run with node --test web/presentation.test.js. No browser dependencies.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const view = require('./presentation.js');

test('expression entry accepts only a literal argument to the selected function', () => {
  const expression = 'twenty-two plus "eleven"\n';
  assert.equal(view.expressionFromCall(`WordAdd.eval(${JSON.stringify(expression)})`), expression);
  assert.equal(view.expressionFromCall('WordAdd.eval(text="one plus two")'), 'one plus two');
  for (const call of ['Other.eval("one plus two")', 'WordAdd.eval("one", extra=2)', 'WordAdd.eval(danger())', 'WordAdd.eval("one"); Other.eval("two")']) {
    assert.equal(view.expressionFromCall(call), null);
  }
});

test('a blocked caller is different from an evaluation error or returned question mark', () => {
  const missing = { source:'unknown', failed:true };
  assert.equal(view.outcome(missing), '');
  assert.equal(view.outcome({ source:'code', failed:true, steps:[missing] }), 'Waiting for an answer');
  assert.equal(view.outcome({ source:'code', failed:true, error:'invalid number' }), 'Error');
  assert.equal(view.outcome({ source:'code', failed:true, steps:[{failed:true,error:'invalid'}] }), 'Stopped by a smaller step');
  assert.equal(view.outcome({ source:'example', output:'"?"', failed:false }), '');
});

test('row navigation uses exact structured inputs, including compound literals and foreign source locations', () => {
  const example = { inputs:['{"name": "a,b", "items": [1, 2]}', '"a\\"b"'], output:'3',path:'other.star',line:20 };
  const f = { qualified:'P.f', inputs:['request','name'], examples:[example] };
  const step = { function:'P.f',input:{request:example.inputs[0],name:example.inputs[1]},call:'a shortened label' };
  assert.deepEqual(view.identity(step,f), { example, call:`P.f(${example.inputs.join(', ')})` });
  assert.equal(view.identity({...step,input:{...step.input,name:'"different"'}},f),null);
});

test('staleness ignores queue changes but detects every source change', () => {
  const files = {'a.star':'original', 'questions.txt':'one'};
  assert.equal(view.projectKey(files),view.projectKey({...files,'questions.txt':'two'}));
  assert.notEqual(view.projectKey(files),view.projectKey({...files,'a.star':'edited'}));
  assert.notEqual(view.projectKey(files),view.projectKey({...files,'b.star':'new'}));
});

test('display values stay text and unknown functions keep their true identity', () => {
  assert.equal(view.readable('"<script>unsafe</script>"'),'<script>unsafe</script>');
  assert.equal(view.title('Custom.evaluate'),'Custom.evaluate');
  assert.equal(view.readable('{"x": None}'),'{"x": None}');
});

test('text answer entry follows homogeneous example columns without changing literal types', () => {
  const f = { inputs:['text'],examples:[{inputs:['"one"'],output:'"alpha"'},{inputs:['"two"'],output:'"beta"'}] };
  assert.equal(view.textOutput(f),true);
  assert.deepEqual(view.editableRow(f,['"one"','"alpha"']),['one','alpha']);
  assert.deepEqual(view.editableRow(f,['" spaced "','""']),['" spaced "','""']);
  assert.deepEqual(view.editableRow(f,['"\\"quoted\\""','"alpha"']),['"\\"quoted\\""','alpha']);
  const mixed = {...f,examples:[...f.examples,{inputs:['None'],output:'3'}]};
  assert.equal(view.textOutput(mixed),false);
  assert.deepEqual(view.editableRow(mixed,['"None"','"3"']),['"None"','"3"']);
});
