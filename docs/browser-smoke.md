# Browser smoke checklist

Run this checklist against the deployed Pi after a release upgrade:

1. Open the root page at desktop width and confirm connection, now-playing,
   station shelf, queue, optional login, and history regions render.
2. Repeat at a 390px phone viewport; confirm controls remain reachable without
   horizontal scrolling and touch targets do not overlap.
3. In open mode, add and remove a harmless direct test stream anonymously.
4. Enter an unknown username, confirm the create-account transition, cancel,
   and verify anonymous operation remains available.
5. In a disposable account, complete password confirmation, log in immediately,
   favorite KFJC, create a personal station/playlist, reload, and verify they
   persist.
6. Open a second browser and verify queue/playback changes arrive through SSE.
7. Disconnect/reconnect the client and confirm its revision reconciles without
   duplicating queue items. Check the browser console for errors.

The desktop, 390px phone, multi-client, anonymous, and account paths were
manually exercised during Milestone 5. The deployed desktop page, live SSE
state, queue, library, and anonymous presentation were rechecked for v0.1.0.
API regressions automate the underlying flows on every test run.
