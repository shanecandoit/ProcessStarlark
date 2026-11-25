const {test}=require("node:test");
const assert=require("node:assert/strict");
const {QuestionFlow}=require("./question-flow.js");
const q=word=>({function:"WordValues.value",input:{word:JSON.stringify(word)},call:`WordValues.value("${word}")`});
test("successive questions keep the original request and show reused facts in another request",()=>{
 const flow=new QuestionFlow();flow.start('WordAdd.eval("twelve plus thirteen")');
 flow.answered(q("twelve"),"12");
 flow.observe({status:"unresolved",why:{steps:[{...q("twelve"),source:"example"},{...q("thirteen"),source:"unknown"}]}});
 assert.equal(flow.original,'WordAdd.eval("twelve plus thirteen")');
 assert.equal(flow.order([q("other"),q("thirteen")])[0].call,q("thirteen").call);
 flow.answered(q("thirteen"),"13");flow.observe({status:"ok",why:{steps:[{...q("twelve"),source:"example"},{...q("thirteen"),source:"example"}]}});
 assert.equal(flow.steps.length,2);assert.equal(flow.reused.length,2);
 flow.start('WordAdd.eval("twelve plus two")');flow.observe({status:"ok",why:{steps:[{...q("twelve"),source:"example"}]}});
 assert.equal(flow.steps.length,0);assert.equal(flow.reused[0].call,q("twelve").call);
 flow.reset();assert.equal(flow.learned.length,0);
});
test("a whole-call authored answer does not claim smaller facts were reused",()=>{
 const flow=new QuestionFlow();flow.start("request");flow.answered(q("twelve"),"12");
 flow.observe({status:"ok",why:{function:"WordAdd.eval",input:{text:'"twelve plus two"'},source:"example"}});
 assert.equal(flow.reused.length,0);
});
