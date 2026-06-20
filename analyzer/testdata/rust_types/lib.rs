pub struct Config {
    pub host: String,
    pub port: u16,
    timeout: Duration,
}

struct InternalState {
    count: usize,
}

pub enum Color {
    Red,
    Green,
    Blue,
}

pub trait Drawable {
    fn draw(&self);
    fn bounds(&self) -> Rect;
}

pub trait Clickable: Drawable {
    fn on_click(&self);
}

pub struct Button {
    pub label: String,
    pub x: i32,
    pub y: i32,
}

impl Drawable for Button {
    fn draw(&self) {
        // render
    }

    fn bounds(&self) -> Rect {
        Rect::new(self.x, self.y, 100, 30)
    }
}

impl Clickable for Button {
    fn on_click(&self) {
        println!("clicked {}", self.label);
    }
}

impl Button {
    pub fn new(label: String) -> Self {
        Button { label, x: 0, y: 0 }
    }

    fn internal_helper(&self) {}
}

pub trait Serializable {
    fn serialize(&self) -> Vec<u8>;
}

impl Serializable for Config {
    fn serialize(&self) -> Vec<u8> {
        vec![]
    }
}
