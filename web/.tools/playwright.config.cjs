const {defineConfig}=require("@playwright/test");
module.exports=defineConfig({
 testDir:"browser",testMatch:"*.spec.cjs",workers:1,fullyParallel:false,
 timeout:60000,expect:{timeout:15000},forbidOnly:!!process.env.CI,
 outputDir:"test-results",reporter:"list",
 use:{headless:true,viewport:{width:1280,height:1000},trace:"retain-on-failure"},
});
