const {test:base,expect}=require("@playwright/test");
const {spawn}=require("node:child_process");
const path=require("node:path");
const root=path.resolve(__dirname,"../../..");
const test=base.extend({
 // eslint-disable-next-line no-empty-pattern -- The isolated server fixture has no dependencies.
 baseURL:async({},use)=>{
  const server=spawn(path.join(__dirname,"../.bin/park-drafts"),["--root",path.join(root,"examples/shipping"),"--db","","--web-dir",path.join(root,"web/dist"),"--addr","127.0.0.1:0"],{stdio:["ignore","pipe","pipe"]});
  let output="";
  try {
   const url=await new Promise((resolve,reject)=>{
    const timer=setTimeout(()=>reject(new Error("server startup timeout: "+output)),15000);
    server.once("error",error=>{clearTimeout(timer);reject(error);});
    server.once("exit",code=>{clearTimeout(timer);reject(new Error(`server exited ${code}: ${output}`));});
    server.stderr.on("data",b=>{output+=b;});
    server.stdout.on("data",b=>{output+=b;const match=/Local draft API: (http:\/\/[^\s]+)/.exec(output);if (match){clearTimeout(timer);resolve(match[1]);}});
   });
   await use(url);
  } finally {
   if (server.pid && server.exitCode===null && server.signalCode===null) await new Promise(resolve=>{server.once("exit",resolve);server.kill("SIGTERM");});
  }
 },
});
test.beforeEach(async({page})=>{
 await page.route("https://cdnjs.cloudflare.com/**",route=>route.abort());
 await page.goto("/");await expect(page.locator("#loading")).toBeHidden();
});
async function shippingDraft(page) {
 await page.locator("#sample").selectOption({label:"Shipping quotes"});
 await expect(page.locator('[data-fn="Shipping.quote"]')).toBeVisible();
 await page.locator("#draft-panel > summary").click();
 await page.locator("#draft-author").fill("developer");
 await page.locator("#draft-create").click();
 await expect(page.locator("#draft-message")).toContainText("Saved");
 await expect(page.locator("#draft-save-state")).toContainText("source saved");
 await page.locator("#release-panel > summary").click();
 await expect(page.locator("#release-request")).toBeEnabled();
}
async function requestReview(page) {
 await page.locator("#release-request").click();
 await expect(page.locator("#release-message")).toContainText("pinned for review");
 await expect(page.locator("#release-review-state")).toContainText("Pending review");
}
async function approve(page) {
 await page.locator("#draft-author").fill("reviewer");
 await expect(page.locator("#release-approve")).toBeEnabled();
 await page.locator("#release-approve").click();
 await expect(page.locator("#release-message")).toContainText("activated atomically");
}
async function addCountry(page,country) {
 const table=page.locator('[data-fn="Zones.zone"]');
 await table.getByRole("textbox",{name:"country for a new row of Zones.zone"}).fill(country);
 await table.getByRole("textbox",{name:"result for a new row of Zones.zone"}).fill("international");
 await table.getByRole("textbox",{name:"result for a new row of Zones.zone"}).press("Enter");
 await expect(table.locator("tr.row").filter({hasText:country})).toHaveCount(1);
 await expect(page.locator("#draft-save")).toBeEnabled();
 await page.locator("#draft-save").click();
 await expect(page.locator("#draft-save-state")).toContainText("source saved");
 await expect(page.locator("#release-request")).toBeEnabled();
}

test("save, inspect, compare, approve, evaluate, reject, reload, and roll back",async({page})=>{
 const errors=[];page.on("pageerror",error=>errors.push(error.message));
 await shippingDraft(page);await requestReview(page);
 await expect(page.locator("#release-checks")).toContainText("3 implementation checks");
 await expect(page.locator("#release-approve")).toBeDisabled();
 await approve(page);await expect(page.locator("#release-production")).toContainText("Generation 1");
 await page.locator("#draft-author").fill("developer");
 await addCountry(page,"DE");await requestReview(page);
 await expect(page.locator("#release-function-changes")).toContainText("Shipping.quote");
 await page.locator("#release-source-changes summary").filter({hasText:"zones.star"}).click();
 await expect(page.locator('pre[aria-label="zones.star Reviewed source"]')).toContainText('zone("DE")');
 await page.locator("#release-comparison > summary").click();
 await page.locator("#release-compare").click();
 await expect(page.locator('#release-comparison-rows tr[data-category="newly_answered"]')).toContainText("Germany");
 await expect(page.locator("#release-comparison-rows")).toContainText("2799");
 await page.locator("#release-panel").screenshot({path:test.info().outputPath("review-panel.png")});
 const scenarioText=await page.locator("#release-scenarios").inputValue();
 await page.locator("#release-scenarios").fill(JSON.stringify([{name:"Only US",call:'Shipping.quote("US", 500)'}]));
 await expect(page.locator("#release-comparison-stale")).toContainText("scenario set changed");
 await page.locator("#release-scenarios").fill(scenarioText);
 await expect(page.locator("#release-comparison-stale")).toHaveText("");
 const downloadPromise=page.waitForEvent("download");await page.locator("#release-comparison-download").click();
 const download=await downloadPromise;expect(download.suggestedFilename()).toBe("park-comparison.json");
 const chunks=[];for await (const chunk of await download.createReadStream()) chunks.push(chunk);
 const downloaded=JSON.parse(Buffer.concat(chunks).toString("utf8"));
 expect(downloaded.generation).toBe(1);expect(downloaded.rows.some(row=>row.category==="newly_answered")).toBeTruthy();
 await page.locator("#release-live > summary").click();
 await page.locator("#release-live-call").fill('Shipping.quote("DE", 800)');
 await page.locator("#release-live-run").click();await expect(page.locator("#release-live-result")).toContainText("unresolved");
 await approve(page);await expect(page.locator("#release-production")).toContainText("Generation 2");
 await expect(page.locator("#release-comparison-stale")).toContainText("active release changed");
 await page.locator("#release-live-run").click();await expect(page.locator("#release-live-result")).toContainText("2799");
 await page.locator("#draft-author").fill("developer");await requestReview(page);
 await page.locator("#draft-author").fill("reviewer");await page.locator("#release-reason").fill("Need more representative cases");
 await page.locator("#release-reject").click();await expect(page.locator("#release-review-state")).toContainText("rejected");
 await expect(page.locator("#release-production")).toContainText("Generation 2");
 const savedID=await page.locator("#draft-picker").inputValue();
 await page.reload();await expect(page.locator("#loading")).toBeHidden();
 await page.locator("#draft-panel > summary").click();
 await expect(page.locator(`#draft-picker option[value="${savedID}"]`)).toHaveCount(1);
 await page.locator("#draft-picker").selectOption(savedID);await page.locator("#draft-replace-ok").check();await page.locator("#draft-open").click();
 await expect(page.locator("#draft-message")).toContainText("Opened");
 await page.locator("#release-panel > summary").click();await expect(page.locator("#release-production")).toContainText("Generation 2");
 await page.locator("#release-recovery > summary").click();await page.locator("#release-rollback-target").selectOption({index:1});
 await page.locator("#release-reason").fill("Restore baseline");await page.locator("#release-rollback").click();
 await expect(page.locator("#release-production")).toContainText("Generation 3");
 await page.locator("#release-live > summary").click();await page.locator("#release-live-call").fill('Shipping.quote("DE", 800)');
 await page.locator("#release-live-run").click();await expect(page.locator("#release-live-result")).toContainText("unresolved");
 expect(errors).toEqual([]);
});

test("stale review conflicts preserve local edits and explain source/base changes",async({page,request,baseURL})=>{
 await shippingDraft(page);await requestReview(page);
 await page.locator("#draft-author").fill("reviewer");
 const id=await page.locator("#draft-picker").inputValue();
 let response=await request.get(`${baseURL}/api/v1/drafts/${id}`);const draft=(await response.json()).draft;
 response=await request.post(`${baseURL}/api/v1/drafts/${id}/operations`,{data:{author:"other-editor",expected_state:draft.state_token,operation:{kind:"add-example",example:'Zones.zone("DE") == "international"'}}});expect(response.ok()).toBeTruthy();
 await page.locator("#release-approve").click();await expect(page.locator("#release-message")).toContainText("browser edits are intact");
 await expect(page.locator("#release-review-state")).toContainText("Saved source changed");
 await expect(page.locator("#release-approve")).toBeDisabled();
 await page.locator('[data-fn="Zones.zone"]').getByRole("textbox",{name:"country for a new row of Zones.zone"}).fill("JP");
 await page.locator("#release-refresh").click();
 await expect(page.locator('[data-fn="Zones.zone"]').getByRole("textbox",{name:"country for a new row of Zones.zone"})).toHaveValue("JP");
 await expect(page.locator("#release-request")).toBeDisabled();
});

test("multiple missing answers keep the original request and show fact reuse on a phone",async({page})=>{
 await page.setViewportSize({width:390,height:844});
 await page.locator("#expression").fill("twelve plus twelve");
 await page.locator("#runform button[type=submit]").click();
 await expect(page.getByRole("textbox",{name:"What number does “twelve” mean?"})).toBeVisible();
 // The second missing fact is introduced by editing one saved vocabulary row.
 const thirteen=page.locator('[data-fn="WordValues.value"] tr.row').filter({hasText:"thirteen"});
 await thirteen.getByRole("button",{name:/Remove example/}).click();
 await expect(thirteen).toHaveCount(0);
 await page.locator("#expression").fill("twelve plus thirteen");await page.locator("#runform button[type=submit]").click();
 await page.getByRole("textbox",{name:"What number does “twelve” mean?"}).fill("12");
 await page.locator('[data-question]').filter({hasText:"twelve"}).getByRole("button",{name:"Save example",exact:true}).click();
 const next=page.getByRole("textbox",{name:"What number does “thirteen” mean?"});await expect(next).toBeFocused();
 await expect(page.locator("#question-original")).toContainText("twelve plus thirteen");
 await next.fill("13");await page.locator('[data-question]').filter({hasText:"thirteen"}).getByRole("button",{name:"Save example",exact:true}).click();
 await expect(page.locator("#output")).toHaveText("25");await expect(page.locator("#question-progress li")).toHaveCount(2);
 await page.locator("#result").screenshot({path:test.info().outputPath("phone-question-flow.png")});
 await expect(page.locator("#question-reuse")).toContainText("twelve");
 await page.locator("#expression").fill("twelve plus two");await page.locator("#runform button[type=submit]").click();
 await expect(page.locator("#output")).toHaveText("14");await expect(page.locator("#question-reuse")).toContainText("twelve");
 expect(await page.evaluate(()=>globalThis.document.documentElement.scrollWidth<=globalThis.innerWidth)).toBeTruthy();
});


test("a competing activation makes the pending review stale without an automatic retry",async({page,request,baseURL})=>{
 await shippingDraft(page);await requestReview(page);await approve(page);
 await page.locator("#draft-author").fill("developer");await requestReview(page);await page.locator("#draft-author").fill("reviewer");
 const id=await page.locator("#draft-picker").inputValue();
 const saved=(await (await request.get(`${baseURL}/api/v1/drafts/${id}`)).json()).draft;
 const other=await request.post(`${baseURL}/api/v1/drafts/${id}/reviews`,{data:{author:"another-developer",expected_state:saved.state_token,expected_generation:1}});
 expect(other.status()).toBe(201);const review=(await other.json()).review;
 const activation=await request.post(`${baseURL}/api/v1/reviews/${review.id}/approve`,{data:{author:"another-reviewer",expected_generation:1}});
 expect(activation.ok()).toBeTruthy();
 await page.locator("#release-approve").click();
 await expect(page.locator("#release-message")).toContainText("browser edits are intact");
 await expect(page.locator("#release-review-state")).toContainText("active release changed");
 await expect(page.locator("#release-production")).toContainText("Generation 2");
 await expect(page.locator("#release-approve")).toBeDisabled();
});

test("saved-source questions automatically rerun the original request until complete",async({page})=>{
 const thirteen=page.locator('[data-fn="WordValues.value"] tr.row').filter({hasText:"thirteen"});
 await thirteen.getByRole("button",{name:/Remove example/}).click();await expect(thirteen).toHaveCount(0);
 await page.locator("#expression").fill("twelve plus thirteen");
 await page.locator("#draft-panel > summary").click();await page.locator("#draft-create").click();
 await expect(page.locator("#draft-save-state")).toContainText("source saved");
 await page.locator("#draft-queue > summary").click();await expect(page.locator("#draft-run")).toBeEnabled();await page.locator("#draft-run").click();
 const twelve=page.getByRole("textbox",{name:'Server answer for WordValues.value("twelve")'});await expect(twelve).toBeVisible();await twelve.fill("12");
 await page.locator("#draft-questions").getByRole("button",{name:"Save answer",exact:true}).click();
 const next=page.getByRole("textbox",{name:'Server answer for WordValues.value("thirteen")'});await expect(next).toBeFocused();await next.fill("13");
 await page.locator("#draft-questions").getByRole("button",{name:"Save answer",exact:true}).click();
 await expect(page.locator("#draft-server-result")).toContainText("25");await expect(page.locator("#draft-question-count")).toContainText("0 open");
 await expect(page.locator("#draft-message")).toContainText("reused by the original request");
});

test("refresh recovers an activity-only token change before requesting review",async({page,request,baseURL})=>{
 await shippingDraft(page);
 const id=await page.locator("#draft-picker").inputValue();
 const saved=(await (await request.get(`${baseURL}/api/v1/drafts/${id}`)).json()).draft;
 const run=await request.post(`${baseURL}/api/v1/drafts/${id}/operations`,{data:{author:"other-actor",expected_state:saved.state_token,operation:{kind:"run",call:'Shipping.quote("US", 500)'}}});
 expect(run.ok()).toBeTruthy();
 await page.locator("#release-refresh").click();await expect(page.locator("#release-refresh")).toBeEnabled();
 await requestReview(page);
});
