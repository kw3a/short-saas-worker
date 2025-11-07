from PIL import Image, ImageDraw, ImageFont
import os
import random

# Helpers: rounded corners with border
def _apply_rounded_with_border(img: Image.Image, radius: int = 16, border: int = 2, border_color=(0,0,0,255)) -> Image.Image:
    """
    Draw a black border exactly on the outer edge and clip the content inside it,
    avoiding any white halo at the very edge.
    """
    img_rgba = img.convert("RGBA")
    w, h = img_rgba.size

    if border and border > 0:
        # 1) Draw the outer border at the exact image edge
        border_layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
        bdraw = ImageDraw.Draw(border_layer)
        bdraw.rounded_rectangle((0, 0, w-1, h-1), radius=radius, outline=border_color, width=border)

        # 2) Create an inner mask inset by the border so content doesn't reach the edge
        inner = Image.new("L", (w, h), 0)
        idraw = ImageDraw.Draw(inner)
        inset = border
        inner_radius = max(0, radius - border)
        idraw.rounded_rectangle((inset, inset, w-1-inset, h-1-inset), radius=inner_radius, fill=255)

        content = img_rgba.copy()
        content.putalpha(inner)

        # 3) Composite content over the border layer
        out = Image.alpha_composite(border_layer, content)
        return out
    else:
        # No border: just apply rounded alpha mask to full area
        mask = Image.new("L", (w, h), 0)
        mdraw = ImageDraw.Draw(mask)
        mdraw.rounded_rectangle((0, 0, w-1, h-1), radius=radius, fill=255)
        img_rgba.putalpha(mask)
        return img_rgba

# Light (normal) theme comment screenshot generator
def _random_username() -> str:
    prefixes = [
        "throwaway", "user", "anon", "just", "real", "cool", "the",
        "auto", "random", "happy", "sad", "noob", "pro",
    ]
    suffix = str(random.randint(10, 99999))
    return f"{random.choice(prefixes)}{suffix}"

def _random_time_ago() -> str:
    unit = random.choice(["m", "h", "d"])  # minutes, hours, days
    if unit == "m":
        n = random.randint(1, 59)
    elif unit == "h":
        n = random.randint(1, 23)
    else:
        n = random.randint(1, 7)
    return f"{n}{unit} ago"

def _random_likes_text() -> str:
    n = random.randint(1, 5000)
    if n >= 1000:
        return f"{n/1000:.1f}k"
    return f"{n:,}"

def _random_upvotes_text() -> str:
    n = random.randint(1, 99999)
    if n >= 1000:
        return f"{n/1000:.1f}k"
    return str(n)

# Light (normal) theme comment screenshot generator
def generate_reddit_comment(comment, output_path="reddit_comment.png"):
    # Visual configuration (light theme)
    S = 2  # supersampling factor for crisper downscale in video
    width = 756 * S
    margin = 40 * S
    avatar_size = 48 * S
    bg_color = (255, 255, 255)       # light background
    text_color = (28, 28, 28)        # main text (nearly black)
    gray = (120, 124, 126)           # meta gray
    light_gray = (240, 241, 242)     # light gray for elements
    accent_gray = (224, 226, 227)    # thread line / separators

    # Choose dynamic font sizes based on comment length to keep image height reasonable
    text_len = len(comment or "")
    user_sz = 20 * S
    meta_sz = 18 * S
    text_sz = 20 * S
    actions_sz = 18 * S
    line_extra = 6 * S
    if text_len >= 700:  # shrink for long comments
        user_sz = 18 * S
        meta_sz = 16 * S
        text_sz = 16 * S
        actions_sz = 16 * S
        line_extra = 4 * S

    # Load sans-serif fonts (prefer system DejaVu Sans, fallback to Arial)
    try:
        font_user = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", user_sz)
        font_meta = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", meta_sz)
        font_text = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", text_sz)
        font_actions = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", actions_sz)
    except Exception:
        font_user = ImageFont.truetype("arial.ttf", user_sz)
        font_meta = ImageFont.truetype("arial.ttf", meta_sz)
        font_text = ImageFont.truetype("arial.ttf", text_sz)
        font_actions = ImageFont.truetype("arial.ttf", actions_sz)

    # Temporary image (for text measurement)
    img = Image.new("RGB", (width, 1000), bg_color)
    draw = ImageDraw.Draw(img)

    # Avatar and left thread line positions
    line_x = margin - 25 * S
    avatar_x = margin
    avatar_y = margin
    text_x = avatar_x + avatar_size + 12 * S

    # Process comment text (handle newlines and paragraphs)
    paragraphs = comment.split("\n")
    max_text_width = width - (text_x + margin)
    lines = []

    for paragraph in paragraphs:
        if not paragraph.strip():
            lines.append("")  # blank line between paragraphs
            continue

        words = paragraph.split()
        current = ""
        for word in words:
            test = current + (" " if current else "") + word
            if draw.textlength(test, font=font_text) <= max_text_width:
                current = test
            else:
                lines.append(current)
                current = word
        if current:
            lines.append(current)

    # Calculate total height
    line_height = font_text.getbbox("Ay")[3] + line_extra
    total_text_height = len(lines) * line_height
    total_height = margin + avatar_size + 20 + total_text_height + 80  # bottom spacing

    # Create final image with adjusted height
    img = Image.new("RGB", (width, total_height), bg_color)
    draw = ImageDraw.Draw(img)

    # Left vertical line (thread)
    draw.line((line_x, margin, line_x, total_height - 20 * S), fill=accent_gray, width=3 * S)

    # Circular avatar placeholder
    try:
        icon_path = os.path.join(os.path.dirname(__file__), "reddit_icon.png")
        icon = Image.open(icon_path).convert("RGBA")
        icon = icon.resize((avatar_size, avatar_size))
        img.paste(icon, (avatar_x, avatar_y), icon)
    except Exception:
        # Random avatar fill color to simulate different users
        rand_fill = (
            random.randint(60, 200),
            random.randint(60, 200),
            random.randint(60, 200),
        )
        draw.ellipse(
            (avatar_x, avatar_y, avatar_x + avatar_size, avatar_y + avatar_size),
            fill=rand_fill,
            outline=gray,
        )

    # Username and time (randomized)
    username = _random_username()
    time_ago = _random_time_ago()
    draw.text((text_x, avatar_y), username, font=font_user, fill=text_color)
    user_width = draw.textlength(username, font=font_user)
    draw.text((text_x + user_width + 8, avatar_y), f"• {time_ago}", font=font_meta, fill=gray)

    # Comment text
    text_y = avatar_y + avatar_size - 40
    for line in lines:
        draw.text((text_x, text_y), line, font=font_text, fill=text_color)
        text_y += line_height

    # Actions (upvote, reply, etc.)
    actions_y = text_y + 10 * S
    likes = _random_likes_text()
    likes_text = f"▲ {likes}"
    draw.text((text_x, actions_y), likes_text, font=font_actions, fill=text_color)
    # Place action items after the measured width of the likes text
    likes_width = draw.textlength(likes_text, font=font_actions)
    actions = ["Reply", "Give Award", "Share", "..."]
    x = text_x + int(likes_width) + 24 * S
    for action in actions:
        draw.text((x, actions_y), action, font=font_actions, fill=gray)
        x += draw.textlength(action, font=font_actions) + 30

    # Apply rounded corners with NO border for comments
    img = _apply_rounded_with_border(img, radius=16 * S, border=0)
    img.save(output_path)
    print(f"✅ Comment image generated successfully: {output_path}")

# Backwards-compatibility wrapper (was previously dark)
def generate_reddit_comment_dark(username, time_ago, likes, comment, output_path="reddit_comment.png"):
    # Backward-compat: ignore username/time_ago/likes and randomize instead
    return generate_reddit_comment(comment=comment, output_path=output_path)


# New: Title screenshot generator (post header style)
def generate_reddit_title_screenshot(subreddit, title, output_path="reddit_title.png"):
    """
    Generate a light-themed image resembling a Reddit post header with title.
    - subreddit: e.g., r/AskReddit
    - username: author name (without u/)
    - time_ago: e.g., 3h ago
    - upvotes: e.g., 12.3k
    - title: post title (wrapped 1..1000 chars)
    """
    S = 2  # supersampling factor
    width = 756 * S
    margin = 24 * S
    bg_color = (255, 255, 255)
    title_color = (28, 28, 28)
    meta_gray = (120, 124, 126)
    light_gray = (240, 241, 242)

    # Load sans-serif fonts for title (prefer system DejaVu Sans, fallback to Arial)
    try:
        font_meta_bold = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", 18 * S)
        font_meta = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 18 * S)
        font_title = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 28 * S)
    except Exception:
        font_meta_bold = ImageFont.truetype("arial.ttf", 18 * S)
        font_meta = ImageFont.truetype("arial.ttf", 18 * S)
        font_title = ImageFont.truetype("arial.ttf", 28 * S)

    # Pre-measure title wrapping
    temp = Image.new("RGB", (width, 2000 * S), bg_color)
    tdraw = ImageDraw.Draw(temp)
    max_w = width - (margin * 2)
    lines = []
    for paragraph in title.split("\n"):
        if not paragraph.strip():
            lines.append("")
            continue
        words = paragraph.split()
        cur = ""
        for w in words:
            test = (cur + " " + w) if cur else w
            if tdraw.textlength(test, font=font_title) <= max_w:
                cur = test
            else:
                lines.append(cur)
                cur = w
        if cur:
            lines.append(cur)

    line_h = font_title.getbbox("Ay")[3] + 6 * S
    title_h = max(line_h * max(1, len(lines)), 40)
    meta_h = font_meta.getbbox("Ay")[3]
    total_h = margin + meta_h + 10 + title_h + 20 + 1  # +1 for separator

    img = Image.new("RGB", (width, total_h), bg_color)
    draw = ImageDraw.Draw(img)

    # Randomize username, time and upvotes regardless of input values
    username = _random_username()
    time_ago = _random_time_ago()
    upvotes = _random_upvotes_text()

    # Meta row (subreddit • Posted by u/username • time)
    meta_left = margin
    meta_text = f"{subreddit} • Posted by u/{username} • {time_ago}"
    draw.text((meta_left, margin), meta_text, font=font_meta, fill=meta_gray)

    # Title
    y = margin + meta_h + 10 * S
    for line in lines:
        draw.text((margin, y), line, font=font_title, fill=title_color)
        y += line_h

    # Upvote hint and separator
    upvote_text = f"▲ {upvotes}"
    uw = draw.textlength(upvote_text, font=font_meta_bold)
    draw.text((width - margin - uw, margin), upvote_text, font=font_meta_bold, fill=title_color)

    # Bottom separator line
    draw.line((0, total_h - 1, width, total_h - 1), fill=light_gray, width=2 * S)

    # Apply rounded corners with border (scaled with supersampling factor)
    img = _apply_rounded_with_border(img, radius=16 * S, border=2 * S)
    img.save(output_path)
    print(f"✅ Title image generated successfully: {output_path}")


# Example usage
if __name__ == "__main__":
    generate_reddit_comment(
        comment=(
            "This is a test comment intended to reach approximately one thousand characters in total length, "
            "used to verify that the system correctly handles the upper limit of input text without errors, truncation, "
            "or unexpected encoding problems. The content itself is arbitrary filler intended purely for testing. "
            "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Integer vel ligula at est volutpat pretium. "
            "Sed tristique sapien nec augue viverra, non suscipit magna vestibulum. Mauris egestas ligula non orci tempor, "
            "sed efficitur turpis facilisis. Phasellus eu libero et justo aliquet elementum. Vivamus facilisis posuere diam, "
            "sed finibus velit vehicula a. Suspendisse gravida fermentum erat, a cursus sem fermentum non. Ut in nisl nec "
            "lectus finibus tincidunt vel vel metus. Curabitur in consequat urna. Donec vehicula sem at libero suscipit, "
            "vel sollicitudin enim cursus. Etiam non arcu nec lacus suscipit pharetra. Quisque elementum euismod nunc, "
            "et laoreet nunc efficitur nec. Integer ac libero sed felis sodales tincidunt. Sed in est sit amet sapien commodo "
            "dapibus. Nulla facilisi. In hac habitasse platea dictumst. Cras feugiat augue sit amet commodo volutpat. "
            "Vestibulum ante ipsum primis in faucibus orci luctus et ultrices posuere cubilia curae; Donec efficitur justo nec "
            "sem blandit, ut volutpat mi imperdiet. Integer non tortor sed sapien iaculis porttitor id in eros. Pellentesque "
            "id libero eget diam fermentum ullamcorper sit amet in lacus. Donec mollis, libero nec commodo viverra, orci metus "
            "eleifend metus, vel dictum leo eros at metus. Integer vitae justo a erat mattis consequat ut a urna. Sed sit amet "
            "turpis nec magna tempus accumsan vel nec metus. "
        ),
    )
    generate_reddit_title_screenshot(
        subreddit="r/AskReddit",
        title="What is something you learned embarrassingly late in life?",
    )