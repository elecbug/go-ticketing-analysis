# Import necessary modules
import sys

def count_strings_in_file(file_name, string_a, string_b, string_c, count):
    try:
        with open(file_name, 'r') as file:
            content = file.read()
            count_a = content.count(string_a)
            count_b = content.count(string_b)
            count_c = content.count(string_c)
            print(f"Occurrences of '{string_a}': {count_a}")
            print(f"Occurrences of '{string_b}': {count_b}")
            print(f"Occurrences of '{string_c}': {count_c}")
            print(f"Total: {count},{count_a},{count_b},{count_c}")
    except FileNotFoundError:
        print(f"Error: File '{file_name}' not found.")
    except Exception as e:
        print(f"An error occurred: {e}")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python count.py <file_name> <count>")
    else:
        file_name = sys.argv[1]
        string_a = '"status":"success"'
        string_b = '"status":"seat_conflict"'
        string_c = '"error":"Error 1040: Too many connections"'
        count_strings_in_file(file_name, string_a, string_b, string_c, sys.argv[2])