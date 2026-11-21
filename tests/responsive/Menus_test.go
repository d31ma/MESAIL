//go:build responsive

package responsive

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

type menuState struct {
	Activator      string   `json:"activator"`
	ActivatorShown bool     `json:"activatorShown"`
	Open           bool     `json:"open"`
	ExposedItems   []string `json:"exposedItems"`
}

type focusState struct {
	Description   string `json:"description"`
	InMenu        bool   `json:"inMenu"`
	InClosedMenu  bool   `json:"inClosedMenu"`
	OnBody        bool   `json:"onBody"`
	OnActivatorOf string `json:"onActivatorOf"`
}

const (
	menuTestsVariable = "MESAIL_MENU_TESTS"
	menuKeyPause      = 300 * time.Millisecond
	closedMenuTabs    = 50
	shiftModifier     = 8
	spaceKey          = " "
)

const menuStateScript = `(() => JSON.stringify([...document.querySelectorAll('w-menu')].map((menu) => {
  const content = menu.querySelector('.w-menu-content');
  const id = menu.getAttribute('activator') || '';
  const activator = id ? document.getElementById(id) : null;
  const open = menu.classList.contains('open') || menu.hasAttribute('open') || (content !== null && content.classList.contains('open'));
  const items = content ? [...content.querySelectorAll('button, a[href], [role^=menuitem], [tabindex]')] : [];
  const exposed = items.filter((item) => !item.closest('[inert]') && !item.closest('[aria-hidden=true]') && getComputedStyle(item).visibility !== 'hidden' && item.getClientRects().length > 0 && !item.disabled);
  return { activator: id, activatorShown: activator !== null && activator.getClientRects().length > 0, open, exposedItems: exposed.map((item) => (item.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 30)) };
})))()`

const focusStateScript = `(() => {
  const active = document.activeElement;
  const menu = active && active.closest ? active.closest('w-menu') : null;
  const content = active && active.closest ? active.closest('.w-menu-content') : null;
  const open = menu !== null && (menu.classList.contains('open') || menu.hasAttribute('open') || (content !== null && content.classList.contains('open')));
  const owner = [...document.querySelectorAll('w-menu[activator]')].map((m) => document.getElementById(m.getAttribute('activator'))).find((a) => a && a.contains(active));
  const label = active ? (active.getAttribute('aria-label') || (active.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 30)) : '';
  return JSON.stringify({ description: active ? active.tagName.toLowerCase() + (active.id ? '#' + active.id : '') + ' "' + label + '"' : 'none', inMenu: content !== null, inClosedMenu: content !== null && !open, onBody: active === document.body || active === null, onActivatorOf: owner ? owner.id : '' });
})()`

const focusActivatorScript = `(() => {
  const host = document.getElementById(%q);
  const target = host.matches('button, a[href], [tabindex]') ? host : (host.querySelector('button, a[href], [tabindex]') || host);
  target.scrollIntoView({ block: 'center' });
  target.focus();
  return JSON.stringify(document.activeElement === target);
})()`

const blurScript = `(() => { if (document.activeElement) { document.activeElement.blur(); } window.scrollTo(0, 0); return JSON.stringify(true); })()`

func requireMenuTests(t *testing.T) {
	t.Helper()
	requireSuite(t)
	if os.Getenv(menuTestsVariable) != "1" {
		t.Skipf("set %s=1 to run the menu tests (scripts/int/run.sh does)", menuTestsVariable)
	}
}

func menuScreens() []*AppView {
	views := shared.site.Profile.Views
	if len(views) == 0 {
		return []*AppView{nil}
	}
	screens := []*AppView{}
	for index := range views {
		screens = append(screens, &views[index])
	}
	return screens
}

func screenName(view *AppView) string {
	if view == nil {
		return "main"
	}
	return view.MenuLabel
}

func openMenuScreen(t *testing.T, view *AppView, size viewport) *Visit {
	t.Helper()
	return openVisit(t, VisitOptions{Width: size.width, Height: size.height, State: SignedInState, View: view})
}

func evaluate[T any](t *testing.T, visit *Visit, script string) T {
	t.Helper()
	var value T
	text, err := chromedp.Run(visit.context, chromedp.Evaluate[string](script))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("evaluate result %q: %v", text, err)
	}
	return value
}

func menus(t *testing.T, visit *Visit) []menuState {
	t.Helper()
	return evaluate[[]menuState](t, visit, menuStateScript)
}

func focus(t *testing.T, visit *Visit) focusState {
	t.Helper()
	return evaluate[focusState](t, visit, focusStateScript)
}

func pause(visit *Visit) {
	_ = chromedp.Do(visit.context, chromedp.Sleep(menuKeyPause))
}

func pressKey(t *testing.T, visit *Visit, key string) {
	t.Helper()
	if err := chromedp.Do(visit.context, chromedp.KeyEvent(key)); err != nil {
		t.Fatalf("key %q: %v", key, err)
	}
	pause(visit)
}

func pressTab(t *testing.T, visit *Visit) {
	t.Helper()
	if err := visit.PressTab(); err != nil {
		t.Fatalf("tab: %v", err)
	}
	pause(visit)
}

func pressShiftTab(t *testing.T, visit *Visit) {
	t.Helper()
	for _, eventType := range []input.DispatchKeyEventType{input.DispatchKeyEventTypeKeyDown, input.DispatchKeyEventTypeKeyUp} {
		params := input.DispatchKeyEventParams{Type: eventType, Key: "Tab", Code: "Tab", WindowsVirtualKeyCode: 9, NativeVirtualKeyCode: 9, Modifiers: shiftModifier}
		if _, err := chromedp.Call(visit.context, input.DispatchKeyEvent, params); err != nil {
			t.Fatalf("shift tab: %v", err)
		}
	}
	pause(visit)
}

func focusActivator(t *testing.T, visit *Visit, activator string) {
	t.Helper()
	if !evaluate[bool](t, visit, fmt.Sprintf(focusActivatorScript, activator)) {
		t.Fatalf("could not focus the activator #%s", activator)
	}
}

func firstShownActivator(states []menuState) string {
	for _, state := range states {
		if state.ActivatorShown && state.Activator != "" {
			return state.Activator
		}
	}
	return ""
}

func openMenuCount(states []menuState) int {
	count := 0
	for _, state := range states {
		if state.Open {
			count++
		}
	}
	return count
}

func TestClosedMenus(t *testing.T) {
	requireMenuTests(t)
	for _, view := range menuScreens() {
		for _, size := range []viewport{{320, 800}, {375, 812}, {768, 1024}, {1280, 800}} {
			t.Run(fmt.Sprintf("%s/%dpx/%s", screenName(view), size.width, shared.scheme.Value), func(t *testing.T) {
				visit := openMenuScreen(t, view, size)
				for _, state := range menus(t, visit) {
					if !state.Open && len(state.ExposedItems) > 0 {
						t.Errorf("closed menu of #%s exposes items to Tab and screen readers: %v", state.Activator, state.ExposedItems)
					}
				}
				evaluate[bool](t, visit, blurScript)
				for press := 1; press <= closedMenuTabs; press++ {
					if err := visit.PressTab(); err != nil {
						t.Fatalf("tab: %v", err)
					}
					if state := focus(t, visit); state.InClosedMenu {
						t.Errorf("Tab %d lands inside a closed menu: %s", press, state.Description)
					}
				}
				reportConsole(t, visit)
			})
		}
	}
}

func TestMenuKeyboard(t *testing.T) {
	requireMenuTests(t)
	for _, view := range menuScreens() {
		for _, size := range []viewport{{320, 800}, {1280, 800}} {
			name := fmt.Sprintf("%s/%dpx/%s", screenName(view), size.width, shared.scheme.Value)
			activator := firstShownActivator(menus(t, openMenuScreen(t, view, size)))
			if activator == "" {
				t.Logf("%s: no menu button on this screen", name)
				continue
			}
			t.Run(name+"/space-arrow-escape", func(t *testing.T) {
				checkSpaceArrowEscape(t, openMenuScreen(t, view, size), activator)
			})
			t.Run(name+"/tab-leaves", func(t *testing.T) {
				checkTabLeaves(t, openMenuScreen(t, view, size), activator)
			})
			t.Run(name+"/shift-tab-returns", func(t *testing.T) {
				checkShiftTabReturns(t, openMenuScreen(t, view, size), activator)
			})
		}
	}
}

func openWithSpace(t *testing.T, visit *Visit, activator string) {
	t.Helper()
	focusActivator(t, visit, activator)
	pressKey(t, visit, spaceKey)
	if openMenuCount(menus(t, visit)) != 1 {
		t.Fatalf("Space on #%s did not open its menu", activator)
	}
	if state := focus(t, visit); !state.InMenu {
		t.Fatalf("Space on #%s opened the menu but focus is on %s, not on an item", activator, state.Description)
	}
	pressKey(t, visit, kb.ArrowDown)
	if state := focus(t, visit); !state.InMenu {
		t.Fatalf("ArrowDown in the menu of #%s moved focus out of it, to %s", activator, state.Description)
	}
}

func checkSpaceArrowEscape(t *testing.T, visit *Visit, activator string) {
	openWithSpace(t, visit, activator)
	pressKey(t, visit, kb.Escape)
	if openMenuCount(menus(t, visit)) != 0 {
		t.Errorf("Escape did not close the menu of #%s", activator)
	}
	if state := focus(t, visit); state.OnActivatorOf != activator {
		t.Errorf("Escape closed the menu of #%s but focus is on %s, not on its button", activator, state.Description)
	}
	reportConsole(t, visit)
}

func checkTabLeaves(t *testing.T, visit *Visit, activator string) {
	openWithSpace(t, visit, activator)
	pressTab(t, visit)
	if openMenuCount(menus(t, visit)) != 0 {
		t.Errorf("Tab from an item did not close the menu of #%s", activator)
	}
	if state := focus(t, visit); state.InMenu || state.OnBody {
		t.Errorf("Tab from the menu of #%s put focus on %s, not on the next control", activator, state.Description)
	}
}

func checkShiftTabReturns(t *testing.T, visit *Visit, activator string) {
	openWithSpace(t, visit, activator)
	pressShiftTab(t, visit)
	if openMenuCount(menus(t, visit)) != 0 {
		t.Errorf("Shift+Tab from an item did not close the menu of #%s", activator)
	}
	if state := focus(t, visit); state.OnActivatorOf != activator {
		t.Errorf("Shift+Tab from the menu of #%s put focus on %s, not on its button", activator, state.Description)
	}
}
