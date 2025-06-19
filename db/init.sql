CREATE TABLE IF NOT EXISTS seats (
    seat_id INT PRIMARY KEY,
    status VARCHAR(20) NOT NULL DEFAULT 'available',
    user_id INT
);
