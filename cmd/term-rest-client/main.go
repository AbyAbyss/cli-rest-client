package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"term-rest-client/internal/models"
	"term-rest-client/pkg/httpclient"
)

var (
	// Version is set during build
	Version = "dev"
	// BuildTime is set during build
	BuildTime = "unknown"
)

// Theme represents a color theme for the application
type Theme struct {
	Name            string
	BackgroundMain  tcell.Color
	BackgroundPanel tcell.Color
	BackgroundInput tcell.Color
	Border          tcell.Color
	BorderFocus     tcell.Color
	AccentPrimary   tcell.Color
	AccentSuccess   tcell.Color
	AccentWarning   tcell.Color
	AccentMauve     tcell.Color
	TextPrimary     tcell.Color
	TextSecondary   tcell.Color
	TextSelected    tcell.Color
	ButtonText      tcell.Color
	// Hex color strings for text formatting
	HexTextPrimary   string
	HexTextSecondary string
	HexAccentPrimary string
	HexAccentSuccess string
	HexAccentWarning string
	HexAccentMauve   string
	HexError         string
	// Method colors
	MethodGET    string
	MethodPOST   string
	MethodDELETE string
}

// Define themes
var (
	// Original theme (previous colors)
	themeOriginal = Theme{
		Name:             "Original",
		BackgroundMain:   tcell.NewRGBColor(15, 17, 26),    // Deep dark background
		BackgroundPanel:  tcell.NewRGBColor(24, 26, 35),    // Panel background
		BackgroundInput:  tcell.NewRGBColor(24, 26, 35),    // Input field background
		Border:           tcell.NewRGBColor(60, 63, 81),    // Subtle border
		BorderFocus:      tcell.NewRGBColor(88, 166, 255),  // Blue focus border
		AccentPrimary:    tcell.NewRGBColor(88, 166, 255),  // Primary accent (blue)
		AccentSuccess:    tcell.NewRGBColor(80, 250, 123),  // Success green
		AccentWarning:    tcell.NewRGBColor(255, 184, 108), // Warning orange
		AccentMauve:      tcell.NewRGBColor(88, 166, 255),  // Blue for buttons (original)
		TextPrimary:      tcell.ColorWhite,
		TextSecondary:    tcell.ColorLightGray,
		TextSelected:     tcell.NewRGBColor(88, 166, 255), // Blue selected
		ButtonText:       tcell.ColorBlack,
		HexTextPrimary:   "white",
		HexTextSecondary: "gray",
		HexAccentPrimary: "#58a6ff",
		HexAccentSuccess: "#50fa7b",
		HexAccentWarning: "#ffb86c",
		HexAccentMauve:   "#58a6ff",
		HexError:         "red",
		MethodGET:        "green",
		MethodPOST:       "yellow",
		MethodDELETE:     "red",
	}

	// Catppuccin Mocha theme (new modern theme)
	themeCatppuccinMocha = Theme{
		Name:             "Catppuccin Mocha",
		BackgroundMain:   tcell.NewHexColor(0x1e1e2e), // Deep Slate/Navy
		BackgroundPanel:  tcell.NewHexColor(0x1e1e2e), // Same as main
		BackgroundInput:  tcell.NewHexColor(0x313244), // Input field background
		Border:           tcell.NewHexColor(0x585b70), // Muted grey-purple border
		BorderFocus:      tcell.NewHexColor(0xf5c2e7), // Neon Pink focus border
		AccentPrimary:    tcell.NewHexColor(0x89b4fa), // Cyan accent
		AccentSuccess:    tcell.NewHexColor(0xa6e3a1), // Success green
		AccentWarning:    tcell.NewHexColor(0xf9e2af), // Warning yellow
		AccentMauve:      tcell.NewHexColor(0xcba6f7), // Mauve for buttons
		TextPrimary:      tcell.NewHexColor(0xcdd6f4), // Soft white/lavender
		TextSecondary:    tcell.NewHexColor(0xa6adc8), // Muted text
		TextSelected:     tcell.NewHexColor(0xf5c2e7), // Selected text color (Neon Pink)
		ButtonText:       tcell.NewHexColor(0x1e1e2e), // Dark text for buttons
		HexTextPrimary:   "#cdd6f4",
		HexTextSecondary: "#a6adc8",
		HexAccentPrimary: "#89b4fa",
		HexAccentSuccess: "#a6e3a1",
		HexAccentWarning: "#f9e2af",
		HexAccentMauve:   "#cba6f7",
		HexError:         "#f38ba8",
		MethodGET:        "#a6e3a1",
		MethodPOST:       "#f9e2af",
		MethodDELETE:     "#f38ba8",
	}
)

func main() {
	// Start with Catppuccin Mocha theme
	currentTheme := &themeCatppuccinMocha

	app := tview.NewApplication()

	state := &models.AppState{
		Method:    "GET",
		URL:       "https://httpbin.org/get",
		Body:      "",
		ActiveTab: 0,
		FocusIdx:  1, // start focus on URL
	}

	// Create HTTP client
	httpClient := httpclient.NewClient(0)

	// ---------- LEFT: COLLECTIONS TREE ----------

	rootNode := tview.NewTreeNode("My Collections").
		SetColor(currentTheme.TextPrimary)

	makeReqNode := func(label, method, url, body, name string) *tview.TreeNode {
		node := tview.NewTreeNode(label)
		node.SetColor(currentTheme.TextPrimary)
		tmpl := &models.RequestTemplate{
			Method: method,
			URL:    url,
			Body:   body,
			Name:   name,
		}
		node.SetReference(&models.TreeNodeData{
			IsCollection: false,
			Template:     tmpl,
			Name:         name,
		})
		return node
	}

	makeCollectionNode := func(name string) *tview.TreeNode {
		node := tview.NewTreeNode(name)
		node.SetColor(currentTheme.TextSecondary)
		node.SetReference(&models.TreeNodeData{
			IsCollection: true,
			Template:     nil,
			Name:         name,
		})
		return node
	}

	// Add sample collections (will be updated when theme changes)
	authAPI := makeCollectionNode("Auth API")
	authAPI.AddChild(makeReqNode(fmt.Sprintf("[%s]GET[%s] Login", currentTheme.MethodGET, currentTheme.HexTextPrimary),
		"GET", "https://httpbin.org/basic-auth/user/passwd", "", "Login"))

	userService := makeCollectionNode("User Service")
	userService.AddChild(makeReqNode(fmt.Sprintf("[%s]GET[%s] Get All Users", currentTheme.MethodGET, currentTheme.HexTextPrimary),
		"GET", "https://httpbin.org/json", "", "Get All Users"))
	userService.AddChild(makeReqNode(fmt.Sprintf("[%s]POST[%s] Create User", currentTheme.MethodPOST, currentTheme.HexTextPrimary),
		"POST", "https://httpbin.org/post", `{"name": "Aby", "role": "admin"}`, "Create User"))
	userService.AddChild(makeReqNode(fmt.Sprintf("[%s]PUT[%s] Update User", currentTheme.MethodPOST, currentTheme.HexTextPrimary),
		"PUT", "https://httpbin.org/put", `{"id": 1, "active": true}`, "Update User"))

	payment := makeCollectionNode("Payment Gateway")
	payment.AddChild(makeReqNode(fmt.Sprintf("[%s]POST[%s] Charge", currentTheme.MethodPOST, currentTheme.HexTextPrimary),
		"POST", "https://httpbin.org/post", `{"amount": 200, "currency": "INR"}`, "Charge"))

	rootNode.AddChild(authAPI)
	rootNode.AddChild(userService)
	rootNode.AddChild(payment)

	tree := tview.NewTreeView()
	tree.SetRoot(rootNode)
	tree.SetCurrentNode(rootNode)
	tree.SetBorder(true)
	tree.SetTitle(" Collections ")
	tree.SetTitleColor(currentTheme.AccentPrimary)
	tree.SetBackgroundColor(currentTheme.BackgroundMain)
	tree.SetBorderColor(currentTheme.Border)
	tree.SetBorderPadding(0, 0, 1, 1)
	// Customize tree to use text color change instead of background inversion
	// We'll handle this in the SetChangedFunc
	tree.SetChangedFunc(func(node *tview.TreeNode) {
		// Reset all node colors to default
		var resetColors func(n *tview.TreeNode)
		resetColors = func(n *tview.TreeNode) {
			if n == rootNode {
				n.SetColor(currentTheme.TextPrimary)
			} else {
				if ref := n.GetReference(); ref != nil {
					if nodeData, ok := ref.(*models.TreeNodeData); ok {
						if nodeData.IsCollection {
							n.SetColor(currentTheme.TextSecondary)
						} else {
							n.SetColor(currentTheme.TextPrimary)
						}
					}
				}
			}
			for _, child := range n.GetChildren() {
				resetColors(child)
			}
		}
		resetColors(rootNode)

		// Set selected node to accent color
		if node != nil && node != rootNode {
			node.SetColor(currentTheme.TextSelected)
		}
	})

	// ---------- TOP: TAB BAR ----------

	tabBar := tview.NewTextView()
	tabBar.SetDynamicColors(true)
	tabBar.SetTextAlign(tview.AlignLeft)
	tabBar.SetBorder(false)
	tabBar.SetBackgroundColor(currentTheme.BackgroundMain)

	tabs := []string{"Builder", "Params", "Auth", "Headers", "Body", "Pre-request", "Tests"}

	updateTabBar := func() {
		var sb strings.Builder
		for i, name := range tabs {
			if i > 0 {
				sb.WriteString("  ")
			}
			num := i + 1
			if i == state.ActiveTab {
				// Active tab: bold and primary accent color
				sb.WriteString(fmt.Sprintf("[%s::b]%d %s[-]", currentTheme.HexAccentPrimary, num, name))
			} else {
				// Inactive tabs: muted text
				sb.WriteString(fmt.Sprintf("[%s]%d %s[-]", currentTheme.HexTextSecondary, num, name))
			}
		}
		tabBar.SetText(sb.String())
	}
	updateTabBar()

	// ---------- BUILDER: METHOD + URL + SEND ----------

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

	methodDrop := tview.NewDropDown()
	methodDrop.SetLabel(" ")
	methodDrop.SetOptions(methods, func(text string, index int) {
		state.Method = text
	})
	methodDrop.SetCurrentOption(0)
	methodDrop.SetBorder(true)
	methodDrop.SetTitle(" Method ")
	methodDrop.SetTitleColor(currentTheme.AccentPrimary)
	methodDrop.SetBorderColor(currentTheme.Border)
	methodDrop.SetBackgroundColor(currentTheme.BackgroundMain)
	methodDrop.SetFieldBackgroundColor(currentTheme.BackgroundInput)
	methodDrop.SetFieldTextColor(currentTheme.TextPrimary)
	methodDrop.SetBorderPadding(0, 0, 1, 1)

	urlInput := tview.NewInputField()
	urlInput.SetLabel(" ")
	urlInput.SetText(state.URL)
	urlInput.SetChangedFunc(func(text string) {
		state.URL = text
	})
	urlInput.SetBorder(true)
	urlInput.SetTitle(" Request URL ")
	urlInput.SetTitleColor(currentTheme.AccentPrimary)
	urlInput.SetBorderColor(currentTheme.Border)
	urlInput.SetBackgroundColor(currentTheme.BackgroundMain)
	urlInput.SetFieldBackgroundColor(currentTheme.BackgroundInput)
	urlInput.SetFieldTextColor(currentTheme.TextPrimary)
	urlInput.SetBorderPadding(0, 0, 1, 1)

	sendButton := tview.NewButton(" SEND ")
	sendButton.SetBorder(false)
	sendButton.SetLabelColor(currentTheme.ButtonText)
	sendButton.SetBackgroundColor(currentTheme.AccentMauve)
	sendButton.SetBackgroundColorActivated(currentTheme.AccentMauve)

	builderTop := tview.NewFlex()
	builderTop.SetDirection(tview.FlexColumn)
	builderTop.AddItem(methodDrop, 16, 0, true)
	builderTop.AddItem(urlInput, 0, 4, false)
	builderTop.AddItem(sendButton, 10, 0, false)

	// ---------- BUILDER: BODY + RESPONSE ----------

	bodyLabel := tview.NewTextView()
	bodyLabel.SetDynamicColors(true)
	bodyLabel.SetText(fmt.Sprintf("[%s::b]JSON Body", currentTheme.HexTextPrimary))
	bodyLabel.SetBorder(false)
	bodyLabel.SetBackgroundColor(currentTheme.BackgroundMain)

	bodyArea := tview.NewTextArea()
	bodyArea.SetText(state.Body, true)
	bodyArea.SetBorder(true)
	bodyArea.SetTitle(" JSON Body ")
	bodyArea.SetTitleColor(currentTheme.AccentPrimary)
	bodyArea.SetBorderColor(currentTheme.Border)
	bodyArea.SetBackgroundColor(currentTheme.BackgroundMain)
	bodyArea.SetBorderPadding(0, 0, 1, 1)

	bodyPanel := tview.NewFlex()
	bodyPanel.SetDirection(tview.FlexRow)
	bodyPanel.AddItem(bodyLabel, 1, 0, false)
	bodyPanel.AddItem(bodyArea, 0, 1, true)

	responseView := tview.NewTextView()
	responseView.SetDynamicColors(true)
	responseView.SetBorder(true)
	responseView.SetTitle(" Response ")
	responseView.SetTitleColor(currentTheme.AccentSuccess)
	responseView.SetBorderColor(currentTheme.Border)
	responseView.SetBackgroundColor(currentTheme.BackgroundMain)
	responseView.SetScrollable(true)
	responseView.SetWordWrap(true)
	responseView.SetBorderPadding(0, 0, 1, 1)
	responseView.SetText(fmt.Sprintf("[%s]Status: (no request yet)", currentTheme.HexTextSecondary))

	builderBottom := tview.NewFlex()
	builderBottom.SetDirection(tview.FlexColumn)
	builderBottom.AddItem(bodyPanel, 0, 1, true)
	builderBottom.AddItem(responseView, 0, 1, false)

	builderPanel := tview.NewFlex()
	builderPanel.SetDirection(tview.FlexRow)
	builderPanel.AddItem(builderTop, 5, 0, true)
	builderPanel.AddItem(builderBottom, 0, 1, false)

	// placeholder for other tabs
	placeholder := tview.NewTextView()
	placeholder.SetDynamicColors(true)
	placeholder.SetBorder(true)
	placeholder.SetTitle(" Coming Soon ")
	placeholder.SetTitleColor(currentTheme.AccentPrimary)
	placeholder.SetBorderColor(currentTheme.Border)
	placeholder.SetBackgroundColor(currentTheme.BackgroundMain)
	placeholder.SetText(fmt.Sprintf("[%s]This tab is a placeholder.\nBuilder tab (1) is active and functional.", currentTheme.HexAccentWarning))
	placeholder.SetWordWrap(true)
	placeholder.SetBorderPadding(0, 0, 1, 1)

	centerPages := tview.NewPages()
	centerPages.AddPage("builder", builderPanel, true, true)
	centerPages.AddPage("other", placeholder, true, false)

	// ---------- RIGHT MAIN COLUMN ----------

	rightColumn := tview.NewFlex()
	rightColumn.SetDirection(tview.FlexRow)
	rightColumn.SetBackgroundColor(currentTheme.BackgroundMain)
	rightColumn.AddItem(tabBar, 3, 0, false)
	rightColumn.AddItem(centerPages, 0, 1, true)

	// ---------- ROOT LAYOUT ----------

	rootFlex := tview.NewFlex()
	rootFlex.SetDirection(tview.FlexColumn)
	rootFlex.SetBackgroundColor(currentTheme.BackgroundMain)
	rootFlex.AddItem(tree, 30, 0, true)
	rootFlex.AddItem(rightColumn, 0, 3, false)

	// ---------- HELPER FUNCTIONS (needed by applyTheme) ----------

	getMethodColor := func(method string) string {
		switch method {
		case "GET":
			return currentTheme.MethodGET
		case "POST", "PUT", "PATCH":
			return currentTheme.MethodPOST
		case "DELETE":
			return currentTheme.MethodDELETE
		default:
			return currentTheme.HexTextPrimary
		}
	}

	updateNodeLabel := func(node *tview.TreeNode, name string, method string) {
		if method != "" {
			color := getMethodColor(method)
			node.SetText(fmt.Sprintf("[%s]%s[%s] %s", color, method, currentTheme.HexTextPrimary, name))
		} else {
			node.SetText(name)
		}
	}

	// ---------- THEME APPLICATION FUNCTION ----------

	applyTheme := func(theme *Theme) {
		// Apply global styles
		tview.Styles.PrimitiveBackgroundColor = theme.BackgroundMain
		tview.Styles.ContrastBackgroundColor = theme.BackgroundMain
		tview.Styles.MoreContrastBackgroundColor = theme.BackgroundMain
		tview.Styles.BorderColor = theme.Border
		tview.Styles.TitleColor = theme.AccentPrimary
		tview.Styles.GraphicsColor = theme.AccentPrimary
		tview.Styles.PrimaryTextColor = theme.TextPrimary
		tview.Styles.SecondaryTextColor = theme.TextSecondary
		tview.Styles.TertiaryTextColor = theme.TextSecondary
		tview.Styles.InverseTextColor = theme.TextPrimary
		tview.Styles.ContrastSecondaryTextColor = theme.TextSecondary

		// Apply to tree
		tree.SetTitleColor(theme.AccentPrimary)
		tree.SetBackgroundColor(theme.BackgroundMain)
		tree.SetBorderColor(theme.Border)
		rootNode.SetColor(theme.TextPrimary)

		// Apply to tab bar
		tabBar.SetBackgroundColor(theme.BackgroundMain)
		updateTabBar()

		// Apply to method dropdown
		methodDrop.SetTitleColor(theme.AccentPrimary)
		methodDrop.SetBorderColor(theme.Border)
		methodDrop.SetBackgroundColor(theme.BackgroundMain)
		methodDrop.SetFieldBackgroundColor(theme.BackgroundInput)
		methodDrop.SetFieldTextColor(theme.TextPrimary)

		// Apply to URL input
		urlInput.SetTitleColor(theme.AccentPrimary)
		urlInput.SetBorderColor(theme.Border)
		urlInput.SetBackgroundColor(theme.BackgroundMain)
		urlInput.SetFieldBackgroundColor(theme.BackgroundInput)
		urlInput.SetFieldTextColor(theme.TextPrimary)

		// Apply to send button
		sendButton.SetLabelColor(theme.ButtonText)
		sendButton.SetBackgroundColor(theme.AccentMauve)
		sendButton.SetBackgroundColorActivated(theme.AccentMauve)

		// Apply to body label
		bodyLabel.SetText(fmt.Sprintf("[%s::b]JSON Body", theme.HexTextPrimary))
		bodyLabel.SetBackgroundColor(theme.BackgroundMain)

		// Apply to body area
		bodyArea.SetTitleColor(theme.AccentPrimary)
		bodyArea.SetBorderColor(theme.Border)
		bodyArea.SetBackgroundColor(theme.BackgroundMain)

		// Apply to response view
		responseView.SetTitleColor(theme.AccentSuccess)
		responseView.SetBorderColor(theme.Border)
		responseView.SetBackgroundColor(theme.BackgroundMain)
		if responseView.GetText(true) == "" || strings.Contains(responseView.GetText(true), "no request yet") {
			responseView.SetText(fmt.Sprintf("[%s]Status: (no request yet)", theme.HexTextSecondary))
		}

		// Apply to placeholder
		placeholder.SetTitleColor(theme.AccentPrimary)
		placeholder.SetBorderColor(theme.Border)
		placeholder.SetBackgroundColor(theme.BackgroundMain)
		placeholder.SetText(fmt.Sprintf("[%s]This tab is a placeholder.\nBuilder tab (1) is active and functional.", theme.HexAccentWarning))

		// Apply to right column
		rightColumn.SetBackgroundColor(theme.BackgroundMain)

		// Update tree node colors and labels
		var updateNodeColors func(n *tview.TreeNode)
		updateNodeColors = func(n *tview.TreeNode) {
			if n == rootNode {
				n.SetColor(theme.TextPrimary)
			} else {
				if ref := n.GetReference(); ref != nil {
					if nodeData, ok := ref.(*models.TreeNodeData); ok {
						if nodeData.IsCollection {
							n.SetColor(theme.TextSecondary)
						} else {
							n.SetColor(theme.TextPrimary)
							// Update node label with new theme colors
							if nodeData.Template != nil {
								updateNodeLabel(n, nodeData.Name, nodeData.Template.Method)
							}
						}
					}
				}
			}
			for _, child := range n.GetChildren() {
				updateNodeColors(child)
			}
		}
		updateNodeColors(rootNode)
	}

	// Apply initial theme
	applyTheme(currentTheme)

	// ---------- FOCUS MANAGEMENT ----------

	focusables := []tview.Primitive{
		tree,
		urlInput,
		bodyArea,
		responseView,
		sendButton,
	}

	setFocusIdx := func(idx int) {
		if idx < 0 {
			idx = len(focusables) - 1
		}
		if idx >= len(focusables) {
			idx = 0
		}
		state.FocusIdx = idx

		// Reset all borders to default
		tree.SetBorderColor(currentTheme.Border)
		methodDrop.SetBorderColor(currentTheme.Border)
		urlInput.SetBorderColor(currentTheme.Border)
		bodyArea.SetBorderColor(currentTheme.Border)
		responseView.SetBorderColor(currentTheme.Border)

		// Set focus border color for the focused widget
		currentFocus := focusables[state.FocusIdx]
		if currentFocus == tree {
			tree.SetBorderColor(currentTheme.BorderFocus)
		} else if currentFocus == urlInput {
			urlInput.SetBorderColor(currentTheme.BorderFocus)
		} else if currentFocus == bodyArea {
			bodyArea.SetBorderColor(currentTheme.BorderFocus)
		} else if currentFocus == responseView {
			responseView.SetBorderColor(currentTheme.BorderFocus)
		}

		app.SetFocus(focusables[state.FocusIdx])
	}

	// ---------- SEND REQUEST FUNCTION ----------

	send := func() {
		state.Body = bodyArea.GetText()
		state.URL = strings.TrimSpace(urlInput.GetText())

		url := state.URL
		if url == "" {
			responseView.SetText(fmt.Sprintf("[%s]Error: URL is empty", currentTheme.HexError))
			return
		}

		method := state.Method
		body := state.Body

		responseView.SetText(fmt.Sprintf("[%s]Sending request...", currentTheme.HexAccentWarning))

		go func() {
			resp, err := httpClient.SendRequest(method, url, body)
			if err != nil {
				app.QueueUpdateDraw(func() {
					responseView.SetText(fmt.Sprintf("[%s]Error:\n%s", currentTheme.HexError, err))
				})
				return
			}

			formatted := httpclient.FormatResponse(resp)
			app.QueueUpdateDraw(func() {
				responseView.SetText(formatted)
			})
		}()
	}

	// ---------- HELPER FUNCTIONS FOR TREE OPERATIONS ----------

	findParentNode := func(target *tview.TreeNode) *tview.TreeNode {
		var findParent func(node *tview.TreeNode) *tview.TreeNode
		findParent = func(node *tview.TreeNode) *tview.TreeNode {
			for _, child := range node.GetChildren() {
				if child == target {
					return node
				}
				if result := findParent(child); result != nil {
					return result
				}
			}
			return nil
		}
		return findParent(rootNode)
	}

	editNodeName := func(node *tview.TreeNode) {
		data := node.GetReference()
		if data == nil {
			return
		}
		nodeData, ok := data.(*models.TreeNodeData)
		if !ok {
			return
		}

		currentName := nodeData.Name
		if currentName == "" {
			currentName = "New Name"
		}

		inputField := tview.NewInputField()
		inputField.SetLabel("Name: ")
		inputField.SetText(currentName)
		inputField.SetFieldWidth(30)
		inputField.SetFieldBackgroundColor(currentTheme.BackgroundInput)
		inputField.SetFieldTextColor(currentTheme.TextPrimary)
		inputField.SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				newName := strings.TrimSpace(inputField.GetText())
				if newName != "" {
					nodeData.Name = newName
					if nodeData.IsCollection {
						updateNodeLabel(node, newName, "")
					} else {
						nodeData.Template.Name = newName
						updateNodeLabel(node, newName, nodeData.Template.Method)
					}
				}
				app.SetRoot(rootFlex, true)
				app.SetFocus(tree)
			} else if key == tcell.KeyEsc {
				app.SetRoot(rootFlex, true)
				app.SetFocus(tree)
			}
		})

		form := tview.NewForm()
		form.AddFormItem(inputField)
		form.AddButton("Save", func() {
			newName := strings.TrimSpace(inputField.GetText())
			if newName != "" {
				nodeData.Name = newName
				if nodeData.IsCollection {
					updateNodeLabel(node, newName, "")
				} else {
					nodeData.Template.Name = newName
					updateNodeLabel(node, newName, nodeData.Template.Method)
				}
			}
			app.SetRoot(rootFlex, true)
			app.SetFocus(tree)
		})
		form.AddButton("Cancel", func() {
			app.SetRoot(rootFlex, true)
			app.SetFocus(tree)
		})
		form.SetBorder(true)
		form.SetTitle(" Edit Name ")
		form.SetTitleColor(currentTheme.AccentPrimary)
		form.SetBorderColor(currentTheme.Border)
		form.SetBackgroundColor(currentTheme.BackgroundMain)

		app.SetRoot(form, true)
		app.SetFocus(inputField)
	}

	deleteNode := func(node *tview.TreeNode) {
		if node == rootNode {
			return // Can't delete root
		}

		parent := findParentNode(node)
		if parent == nil {
			return
		}

		modal := tview.NewModal()
		modal.SetText(fmt.Sprintf("Delete '%s'?\n\nPress Enter to confirm, Esc to cancel", node.GetText()))
		modal.AddButtons([]string{"Delete", "Cancel"})
		modal.SetBackgroundColor(currentTheme.BackgroundMain)
		modal.SetBorderColor(currentTheme.Border)
		modal.SetTextColor(currentTheme.TextPrimary)
		modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete" {
				parent.RemoveChild(node)
				tree.SetCurrentNode(parent)
			}
			app.SetRoot(rootFlex, true)
			app.SetFocus(tree)
		})

		app.SetRoot(modal, true)
		app.SetFocus(modal)
	}

	saveCurrentRequest := func() {
		// Find or create a collection to save to
		var targetCollection *tview.TreeNode
		if tree.GetCurrentNode() != nil && tree.GetCurrentNode() != rootNode {
			current := tree.GetCurrentNode()
			data := current.GetReference()
			if data != nil {
				if nodeData, ok := data.(*models.TreeNodeData); ok {
					if nodeData.IsCollection {
						targetCollection = current
					} else {
						targetCollection = findParentNode(current)
					}
				}
			}
		}

		if targetCollection == nil || targetCollection == rootNode {
			// Create a new collection or use first available
			if len(rootNode.GetChildren()) > 0 {
				targetCollection = rootNode.GetChildren()[0]
			} else {
				targetCollection = makeCollectionNode("New Collection")
				rootNode.AddChild(targetCollection)
			}
		}

		// Create request name
		reqName := "New Request"
		if state.URL != "" {
			// Try to extract a meaningful name from URL
			parts := strings.Split(strings.Trim(state.URL, "/"), "/")
			if len(parts) > 0 {
				reqName = parts[len(parts)-1]
				if reqName == "" && len(parts) > 1 {
					reqName = parts[len(parts)-2]
				}
			}
		}

		newReq := makeReqNode(
			fmt.Sprintf("[%s]%s[%s] %s", getMethodColor(state.Method), state.Method, currentTheme.HexTextPrimary, reqName),
			state.Method,
			state.URL,
			state.Body,
			reqName,
		)
		targetCollection.AddChild(newReq)
		tree.SetCurrentNode(newReq)
	}

	// ---------- TREE SELECTION (LOAD TEMPLATE) ----------

	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		if ref := node.GetReference(); ref != nil {
			if nodeData, ok := ref.(*models.TreeNodeData); ok && !nodeData.IsCollection && nodeData.Template != nil {
				tmpl := nodeData.Template
				state.Method = tmpl.Method
				state.URL = tmpl.URL
				state.Body = tmpl.Body

				for i, m := range methods {
					if m == tmpl.Method {
						methodDrop.SetCurrentOption(i)
						break
					}
				}
				urlInput.SetText(state.URL)
				bodyArea.SetText(state.Body, true)
				responseView.SetText(fmt.Sprintf("[%s]Loaded request. Press Ctrl+S or SEND.", currentTheme.HexTextSecondary))
				setFocusIdx(1) // URL
			}
		}
	})

	// ---------- WIRE SEND TRIGGERS ----------

	sendButton.SetSelectedFunc(send)
	urlInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			send()
		}
	})

	// ---------- MOUSE SUPPORT (CLICK TO FOCUS) ----------

	// Enable mouse support
	app.EnableMouse(true)

	// Add mouse click handlers for focus
	tree.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			app.SetFocus(tree)
			setFocusIdx(0)
		}
		return action, event
	})

	urlInput.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			app.SetFocus(urlInput)
			setFocusIdx(1)
		}
		return action, event
	})

	bodyArea.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			app.SetFocus(bodyArea)
			setFocusIdx(2)
		}
		return action, event
	})

	responseView.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			app.SetFocus(responseView)
			setFocusIdx(3)
		}
		return action, event
	})

	sendButton.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			app.SetFocus(sendButton)
			setFocusIdx(4)
			send()
		}
		return action, event
	})

	methodDrop.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			app.SetFocus(methodDrop)
		}
		return action, event
	})

	// ---------- GLOBAL KEYBINDINGS ----------

	app.SetRoot(rootFlex, true)

	app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// Quit
		if ev.Key() == tcell.KeyEsc ||
			(ev.Key() == tcell.KeyRune &&
				(ev.Rune() == 'q' || ev.Rune() == 'Q') &&
				(ev.Modifiers()&tcell.ModCtrl != 0)) {
			app.Stop()
			return nil
		}

		// Focus: Tab / Shift+Tab
		if ev.Key() == tcell.KeyTab {
			setFocusIdx(state.FocusIdx + 1)
			return nil
		}
		if ev.Key() == tcell.KeyBacktab {
			setFocusIdx(state.FocusIdx - 1)
			return nil
		}

		// Send: Ctrl+Enter or Ctrl+S
		if ev.Key() == tcell.KeyEnter && (ev.Modifiers()&tcell.ModCtrl != 0) {
			send()
			return nil
		}
		if ev.Key() == tcell.KeyRune &&
			(ev.Rune() == 's' || ev.Rune() == 'S') &&
			(ev.Modifiers()&tcell.ModCtrl != 0) {
			send()
			return nil
		}

		// Tabs: 1–7
		if ev.Key() == tcell.KeyRune && ev.Rune() >= '1' && ev.Rune() <= '7' {
			idx := int(ev.Rune() - '1')
			if idx >= len(tabs) {
				return ev
			}
			state.ActiveTab = idx
			if idx == 0 {
				centerPages.ShowPage("builder")
				centerPages.HidePage("other")
			} else {
				centerPages.ShowPage("other")
				centerPages.HidePage("builder")
			}
			updateTabBar()
			return nil
		}

		// Edit name: 'e' when tree is focused
		if ev.Key() == tcell.KeyRune && (ev.Rune() == 'e' || ev.Rune() == 'E') {
			if app.GetFocus() == tree {
				currentNode := tree.GetCurrentNode()
				if currentNode != nil && currentNode != rootNode {
					editNodeName(currentNode)
					return nil
				}
			}
		}

		// Save current request: Ctrl+Shift+S
		if ev.Key() == tcell.KeyRune &&
			(ev.Rune() == 's' || ev.Rune() == 'S') &&
			(ev.Modifiers()&tcell.ModCtrl != 0) &&
			(ev.Modifiers()&tcell.ModShift != 0) {
			saveCurrentRequest()
			return nil
		}

		// Delete node: Delete or 'd' when tree is focused
		if ev.Key() == tcell.KeyDelete || (ev.Key() == tcell.KeyRune && (ev.Rune() == 'd' || ev.Rune() == 'D')) {
			if app.GetFocus() == tree {
				currentNode := tree.GetCurrentNode()
				if currentNode != nil && currentNode != rootNode {
					deleteNode(currentNode)
					return nil
				}
			}
		}

		// Create new collection: 'n' when tree is focused
		if ev.Key() == tcell.KeyRune && (ev.Rune() == 'n' || ev.Rune() == 'N') {
			if app.GetFocus() == tree {
				newCollection := makeCollectionNode("New Collection")
				rootNode.AddChild(newCollection)
				tree.SetCurrentNode(newCollection)
				editNodeName(newCollection)
				return nil
			}
		}

		return ev
	})

	// initial focus
	setFocusIdx(state.FocusIdx)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
