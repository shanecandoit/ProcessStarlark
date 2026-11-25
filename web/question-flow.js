// Session-scoped progress through the missing facts behind an original request.
(function(root) {
  "use strict";
  const identity=q=>q.function+JSON.stringify(Object.entries(q.input || {}).sort(([a],[b])=>a.localeCompare(b)));
  class QuestionFlow {
    constructor() {this.reset();}
    reset() {this.original="";this.steps=[];this.learned=[];this.reused=[];this.current=null;}
    start(call) {this.original=call;this.steps=[];this.reused=[];this.current=null;}
    answered(question,output) {
      const fact={call:question.call,output,key:identity(question)};
      this.steps.push(fact);
      this.learned=this.learned.filter(f=>f.key!==fact.key);this.learned.push(fact);
    }
    observe(result) {
      this.current=null;const used=new Set();
      function walk(step) {
        if (!step) return;
        if (step.source==="example") used.add(identity(step));
        for (const child of step.steps || []) walk(child);
      }
      walk(result.why);
      this.reused=this.learned.filter(f=>used.has(f.key));
      const find=step=>{
        if (!step) return null;
        if (step.source==="unknown" || step.source==="unconfirmed code") return step;
        for (const child of step.steps || []) {const found=find(child);if (found) return found;}
        return null;
      };
      if (["unresolved","unconfirmed"].includes(result.status)) this.current=find(result.why);
    }
    order(questions) {
      if (!this.current) return questions;
      const key=identity(this.current);
      return [...questions].sort((a,b)=>Number(identity(b)===key)-Number(identity(a)===key));
    }
  }
  root.ParkQuestionFlow={QuestionFlow};
  if (typeof module!=="undefined" && module.exports) module.exports={QuestionFlow};
})(globalThis);
