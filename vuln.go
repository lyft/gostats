package vuln
import (
    "fmt"
    "os"
)
func main(){
    q := fmt.Sprintf("SELECT * FROM foo where name = '%s'", os.Args[1])
}
